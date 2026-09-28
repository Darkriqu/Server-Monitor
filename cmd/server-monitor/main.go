package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Darkriqu/Server-Monitor/internal/alerts"
	"github.com/Darkriqu/Server-Monitor/internal/api"
	"github.com/Darkriqu/Server-Monitor/internal/collector"
	"github.com/Darkriqu/Server-Monitor/internal/config"
	"github.com/Darkriqu/Server-Monitor/internal/store"
	webassets "github.com/Darkriqu/Server-Monitor/web"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "", "path to TOML config")
	showVersion := flag.Bool("version", false, "print version")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("configuration error", "error", err)
		os.Exit(2)
	}
	if err := os.MkdirAll(cfg.DataDir, 0750); err != nil {
		slog.Error("data directory", "error", err)
		os.Exit(2)
	}
	cap := int(cfg.History/cfg.Interval) + 2
	ring := store.NewRing(cap)
	var db *store.SQLite
	if cfg.Persistence.Enabled {
		path := cfg.Persistence.Path
		if !filepath.IsAbs(path) {
			path = filepath.Clean(path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0750); err != nil {
			slog.Error("database directory", "error", err)
			os.Exit(2)
		}
		db, err = store.OpenSQLite(path, cfg.Persistence.BatchSize)
		if err != nil {
			slog.Error("sqlite", "error", err)
			os.Exit(2)
		}
		defer db.Close()
	}
	sys, err := collector.System()
	if err != nil {
		slog.Error("system metadata", "error", err)
		os.Exit(2)
	}
	token := cfg.Auth.Token
	cfg.Auth.Token = ""
	am := alerts.New(cfg)
	server := api.New(sys, ring, db, am, token, webassets.Files())
	token = ""
	httpSrv := &http.Server{Addr: cfg.Listen, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 75 * time.Second}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	go collectLoop(ctx, cfg, ring, db, am, server)
	go func() {
		<-ctx.Done()
		server.CloseStreams()
		shutdownCtx, c := context.WithTimeout(context.Background(), 8*time.Second)
		defer c()
		if db != nil {
			_ = db.Flush()
		}
		_ = api.Shutdown(shutdownCtx, httpSrv)
	}()
	slog.Info("server monitor started", "listen", cfg.Listen, "interval", cfg.Interval, "history", cfg.History, "persistence", cfg.Persistence.Enabled)
	if cfg.TLS.Enabled {
		err = httpSrv.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile)
	} else {
		err = httpSrv.ListenAndServe()
	}
	if err != nil && err != http.ErrServerClosed {
		slog.Error("http server", "error", err)
		os.Exit(1)
	}
}

func collectLoop(ctx context.Context, cfg config.Config, ring *store.Ring, db *store.SQLite, am *alerts.Manager, s *api.Server) {
	c := collector.New()
	interval := cfg.Interval
	next := time.Now().Truncate(interval).Add(interval)
	timer := time.NewTimer(time.Until(next))
	defer timer.Stop()
	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()
	for {
		select {
		case now := <-timer.C:
			snap, err := c.Collect(now)
			if err != nil {
				slog.Warn("collect failed", "error", err)
			} else {
				ring.Add(snap)
				am.Evaluate(snap)
				s.Broadcast(snap)
				if db != nil {
					if err := db.Queue(snap); err != nil {
						slog.Warn("persist failed", "error", err)
					}
				}
			}
			next = next.Add(interval)
			for !next.After(time.Now()) {
				next = next.Add(interval)
			}
			timer.Reset(time.Until(next))
		case <-cleanup.C:
			if db != nil {
				if err := db.Compact(ctx); err != nil {
					slog.Warn("compact failed", "error", err)
				}
				_ = db.Cleanup(ctx)
			}
		case <-ctx.Done():
			return
		}
	}
}
