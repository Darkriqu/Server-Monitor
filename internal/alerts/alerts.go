package alerts

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/Darkriqu/Server-Monitor/internal/config"
	"github.com/Darkriqu/Server-Monitor/internal/model"
)

type state struct {
	since      time.Time
	active     bool
	lastNotify time.Time
}
type Manager struct {
	mu      sync.RWMutex
	cfg     config.Config
	states  map[string]*state
	history []model.Alert
	client  *http.Client
}

func New(c config.Config) *Manager {
	return &Manager{cfg: c, states: map[string]*state{}, history: make([]model.Alert, 0, 256), client: &http.Client{Timeout: 5 * time.Second}}
}
func (m *Manager) History() []model.Alert {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := append([]model.Alert(nil), m.history...)
	return out
}
func (m *Manager) Evaluate(s model.Snapshot) {
	now := s.Timestamp
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if s.CPU.Total != nil {
		m.eval("cpu", *s.CPU.Total, m.cfg.Alerts.CPU, now)
	}
	m.eval("memory", s.Memory.UsedPct, m.cfg.Alerts.Memory, now)
	if s.Memory.SwapTotal > 0 {
		m.eval("swap", float64(s.Memory.SwapUsed)*100/float64(s.Memory.SwapTotal), m.cfg.Alerts.Swap, now)
	}
	maxDisk := 0.0
	for _, d := range s.Mounts {
		if d.UsedPct > maxDisk {
			maxDisk = d.UsedPct
		}
	}
	if len(s.Mounts) > 0 {
		m.eval("disk", maxDisk, m.cfg.Alerts.Disk, now)
	}
}
func (m *Manager) eval(name string, value float64, r config.Rule, now time.Time) {
	m.mu.Lock()
	st := m.states[name]
	if st == nil {
		st = &state{}
		m.states[name] = st
	}
	if !st.active {
		if value > r.Threshold {
			if st.since.IsZero() {
				st.since = now
			}
			if now.Sub(st.since) >= r.For {
				st.active = true
				st.lastNotify = now
				a := mkAlert(name, "firing", value, r.Threshold, st.since, now)
				m.push(a)
				m.mu.Unlock()
				go m.send(a)
				return
			}
		} else {
			st.since = time.Time{}
		}
	} else {
		if value < r.Threshold-r.Hysteresis {
			a := mkAlert(name, "resolved", value, r.Threshold, st.since, now)
			st.active = false
			st.since = time.Time{}
			m.push(a)
			m.mu.Unlock()
			go m.send(a)
			return
		}
		if now.Sub(st.lastNotify) >= r.Cooldown {
			st.lastNotify = now
			a := mkAlert(name, "firing", value, r.Threshold, st.since, now)
			m.push(a)
			m.mu.Unlock()
			go m.send(a)
			return
		}
	}
	m.mu.Unlock()
}
func mkAlert(rule, state string, value, threshold float64, start, now time.Time) model.Alert {
	id := fmt.Sprintf("%s-%d", rule, start.Unix())
	return model.Alert{ID: id, Rule: rule, State: state, Value: value, Threshold: threshold, StartedAt: start, UpdatedAt: now, Message: fmt.Sprintf("%s %s: %.1f%% (threshold %.1f%%)", rule, state, value, threshold)}
}
func (m *Manager) push(a model.Alert) {
	m.history = append(m.history, a)
	if len(m.history) > 1000 {
		m.history = append([]model.Alert(nil), m.history[len(m.history)-1000:]...)
	}
}
func (m *Manager) send(a model.Alert) {
	if u := m.cfg.Alerts.WebhookURL; u != "" {
		b, _ := json.Marshal(a)
		req, _ := http.NewRequest(http.MethodPost, u, bytes.NewReader(b))
		req.Header.Set("Content-Type", "application/json")
		if resp, err := m.client.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}
	if tok, chat := m.cfg.Alerts.TelegramBotToken, m.cfg.Alerts.TelegramChatID; tok != "" && chat != "" {
		endpoint := "https://api.telegram.org/bot" + tok + "/sendMessage"
		v := url.Values{"chat_id": {chat}, "text": {a.Message}}
		if resp, err := m.client.PostForm(endpoint, v); err == nil {
			_ = resp.Body.Close()
		}
	}
}
