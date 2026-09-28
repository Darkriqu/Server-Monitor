package api

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Darkriqu/Server-Monitor/internal/alerts"
	"github.com/Darkriqu/Server-Monitor/internal/model"
	"github.com/Darkriqu/Server-Monitor/internal/store"
)

type Server struct {
	system        model.SystemInfo
	ring          *store.Ring
	db            *store.SQLite
	alerts        *alerts.Manager
	token         [32]byte
	assets        fs.FS
	mux           *http.ServeMux
	subscribersMu sync.Mutex
	subscribers   map[chan model.Snapshot]struct{}
}

func New(system model.SystemInfo, ring *store.Ring, db *store.SQLite, am *alerts.Manager, token string, assets fs.FS) *Server {
	s := &Server{system: system, ring: ring, db: db, alerts: am, token: sha256.Sum256([]byte(token)), assets: assets, mux: http.NewServeMux(), subscribers: map[chan model.Snapshot]struct{}{}}
	s.routes()
	return s
}
func (s *Server) Handler() http.Handler { return security(s.rateLimit(s.auth(s.mux))) }
func (s *Server) Broadcast(v model.Snapshot) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	for ch := range s.subscribers {
		select {
		case ch <- v:
		default:
		}
	}
}
func (s *Server) CloseStreams() {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()
	for ch := range s.subscribers {
		close(ch)
		delete(s.subscribers, ch)
	}
}
func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.health)
	s.mux.HandleFunc("GET /api/v1/system", s.systemInfo)
	s.mux.HandleFunc("GET /api/v1/metrics", s.metrics)
	s.mux.HandleFunc("GET /api/v1/processes", s.processes)
	s.mux.HandleFunc("GET /api/v1/alerts", s.alertList)
	s.mux.HandleFunc("GET /api/v1/ws", s.stream)
	s.mux.Handle("/", http.FileServerFS(s.assets))
}
func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"status": "ok", "time": time.Now().UTC()})
}
func (s *Server) systemInfo(w http.ResponseWriter, r *http.Request) {
	si := s.system
	if up := time.Now().Unix() - si.BootTime; up > 0 {
		si.UptimeSec = uint64(up)
	}
	writeJSON(w, si)
}
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	now := time.Now().UTC()
	from := parseTime(r.URL.Query().Get("from"), now.Add(-5*time.Minute))
	to := parseTime(r.URL.Query().Get("to"), now)
	step := parseDuration(r.URL.Query().Get("step"))
	data := s.ring.Query(from, to, step)
	if s.db != nil && len(data) == 0 && now.Sub(from) > time.Hour {
		if rows, err := s.db.Load(r.Context(), from, to, step); err == nil {
			data = rows
		}
	}
	writeJSON(w, map[string]any{"from": from, "to": to, "step_seconds": step.Seconds(), "metrics": data})
}
func (s *Server) processes(w http.ResponseWriter, r *http.Request) {
	v, ok := s.ring.Latest()
	if !ok {
		writeJSON(w, map[string]any{"process_count": 0, "top_cpu": []any{}, "top_ram": []any{}})
		return
	}
	writeJSON(w, map[string]any{"process_count": v.ProcessCount, "top_cpu": v.TopCPU, "top_ram": v.TopRAM})
}
func (s *Server) alertList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"alerts": s.alerts.History()})
}
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", 500)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	ch := make(chan model.Snapshot, 8)
	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()
	defer func() { s.subscribersMu.Lock(); delete(s.subscribers, ch); s.subscribersMu.Unlock() }()
	ping := time.NewTicker(20 * time.Second)
	defer ping.Stop()
	for {
		select {
		case v, ok := <-ch:
			if !ok {
				return
			}
			b, _ := json.Marshal(v)
			fmt.Fprintf(w, "event: metrics\ndata: %s\n\n", b)
			fl.Flush()
		case <-ping.C:
			fmt.Fprint(w, ": ping\n\n")
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
func (s *Server) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" || (!strings.HasPrefix(r.URL.Path, "/api/") && r.Method == http.MethodGet) {
			next.ServeHTTP(w, r)
			return
		}
		raw := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		got := sha256.Sum256([]byte(raw))
		if raw == "" || subtle.ConstantTimeCompare(got[:], s.token[:]) != 1 {
			w.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func (s *Server) rateLimit(next http.Handler) http.Handler {
	type entry struct {
		n     int
		reset time.Time
	}
	var mu sync.Mutex
	m := map[string]*entry{}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		key := r.RemoteAddr
		now := time.Now()
		mu.Lock()
		e := m[key]
		if e == nil || now.After(e.reset) {
			e = &entry{reset: now.Add(time.Minute)}
			m[key] = e
		}
		e.n++
		allowed := e.n <= 600
		mu.Unlock()
		if !allowed {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
func security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Content-Security-Policy", "default-src 'self'; connect-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'")
		next.ServeHTTP(w, r)
	})
}
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Debug("write json", "error", err)
	}
}
func parseTime(v string, fallback time.Time) time.Time {
	if v == "" {
		return fallback
	}
	if n, err := strconv.ParseInt(v, 10, 64); err == nil {
		return time.Unix(n, 0).UTC()
	}
	if t, err := time.Parse(time.RFC3339, v); err == nil {
		return t
	}
	return fallback
}
func parseDuration(v string) time.Duration {
	if v == "" {
		return 0
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * time.Second
	}
	return 0
}
func Shutdown(ctx context.Context, srv *http.Server) error { return srv.Shutdown(ctx) }
