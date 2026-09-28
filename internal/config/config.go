package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

type Rule struct {
	Threshold    float64       `toml:"threshold"`
	For          time.Duration `toml:"-"`
	ForText      string        `toml:"for"`
	Hysteresis   float64       `toml:"hysteresis"`
	Cooldown     time.Duration `toml:"-"`
	CooldownText string        `toml:"cooldown"`
}

type Config struct {
	Listen       string        `toml:"listen"`
	Interval     time.Duration `toml:"-"`
	IntervalText string        `toml:"interval"`
	History      time.Duration `toml:"-"`
	HistoryText  string        `toml:"history"`
	DataDir      string        `toml:"data_dir"`
	Auth         struct {
		Token string `toml:"token"`
	} `toml:"auth"`
	Persistence struct {
		Enabled   bool   `toml:"enabled"`
		Path      string `toml:"path"`
		BatchSize int    `toml:"batch_size"`
	} `toml:"persistence"`
	Alerts struct {
		CPU              Rule   `toml:"cpu"`
		Memory           Rule   `toml:"memory"`
		Disk             Rule   `toml:"disk"`
		Swap             Rule   `toml:"swap"`
		WebhookURL       string `toml:"webhook_url"`
		TelegramBotToken string `toml:"telegram_bot_token"`
		TelegramChatID   string `toml:"telegram_chat_id"`
	} `toml:"alerts"`
	TLS struct {
		Enabled  bool   `toml:"enabled"`
		CertFile string `toml:"cert_file"`
		KeyFile  string `toml:"key_file"`
	} `toml:"tls"`
}

func Default() Config {
	var c Config
	c.Listen = "127.0.0.1:9090"
	c.Interval = time.Second
	c.IntervalText = "1s"
	c.History = 24 * time.Hour
	c.HistoryText = "24h"
	c.DataDir = "./data"
	c.Persistence.Path = "./data/metrics.db"
	c.Persistence.BatchSize = 60
	c.Alerts.CPU = defaultRule(90)
	c.Alerts.Memory = defaultRule(90)
	c.Alerts.Disk = defaultRule(90)
	c.Alerts.Swap = defaultRule(80)
	return c
}

func defaultRule(th float64) Rule {
	return Rule{Threshold: th, For: 2 * time.Minute, ForText: "2m", Hysteresis: 5, Cooldown: 15 * time.Minute, CooldownText: "15m"}
}

func Load(path string) (Config, error) {
	c := Default()
	if path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return c, err
		}
		if err := toml.Unmarshal(b, &c); err != nil {
			return c, err
		}
	}
	if v := os.Getenv("SERVER_MONITOR_TOKEN"); v != "" {
		c.Auth.Token = v
	}
	if v := os.Getenv("SERVER_MONITOR_LISTEN"); v != "" {
		c.Listen = v
	}
	if v := os.Getenv("SERVER_MONITOR_INTERVAL"); v != "" {
		c.IntervalText = v
	}
	if v := os.Getenv("SERVER_MONITOR_HISTORY"); v != "" {
		c.HistoryText = v
	}
	if v := os.Getenv("SERVER_MONITOR_DATA_DIR"); v != "" {
		c.DataDir = v
	}
	if v := os.Getenv("SERVER_MONITOR_PERSISTENCE"); v != "" {
		b, _ := strconv.ParseBool(v)
		c.Persistence.Enabled = b
	}
	if err := parseDurations(&c); err != nil {
		return c, err
	}
	if c.Auth.Token == "" {
		return c, errors.New("auth token is required: set auth.token or SERVER_MONITOR_TOKEN")
	}
	if c.Interval < 250*time.Millisecond || c.Interval > 60*time.Second {
		return c, fmt.Errorf("interval must be between 250ms and 60s")
	}
	if c.History < c.Interval {
		return c, fmt.Errorf("history must be >= interval")
	}
	if !strings.Contains(c.Listen, ":") {
		return c, fmt.Errorf("listen must be host:port")
	}
	return c, nil
}

func parseDurations(c *Config) error {
	var err error
	if c.Interval, err = time.ParseDuration(c.IntervalText); err != nil {
		return fmt.Errorf("interval: %w", err)
	}
	if c.History, err = time.ParseDuration(c.HistoryText); err != nil {
		return fmt.Errorf("history: %w", err)
	}
	for name, r := range map[string]*Rule{"cpu": &c.Alerts.CPU, "memory": &c.Alerts.Memory, "disk": &c.Alerts.Disk, "swap": &c.Alerts.Swap} {
		if r.ForText == "" {
			r.ForText = "2m"
		}
		if r.CooldownText == "" {
			r.CooldownText = "15m"
		}
		if r.For, err = time.ParseDuration(r.ForText); err != nil {
			return fmt.Errorf("alerts.%s.for: %w", name, err)
		}
		if r.Cooldown, err = time.ParseDuration(r.CooldownText); err != nil {
			return fmt.Errorf("alerts.%s.cooldown: %w", name, err)
		}
	}
	return nil
}
