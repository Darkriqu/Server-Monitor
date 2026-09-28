package alerts

import (
	"github.com/Darkriqu/Server-Monitor/internal/config"
	"github.com/Darkriqu/Server-Monitor/internal/model"
	"testing"
	"time"
)

func TestHysteresis(t *testing.T) {
	c := config.Default()
	c.Alerts.CPU.Threshold = 80
	c.Alerts.CPU.For = 0
	c.Alerts.CPU.Hysteresis = 5
	c.Alerts.CPU.Cooldown = time.Hour
	m := New(c)
	v := 90.0
	now := time.Now()
	m.Evaluate(model.Snapshot{Timestamp: now, CPU: model.CPU{Total: &v}})
	if len(m.History()) != 1 {
		t.Fatal("expected firing")
	}
	v = 77
	m.Evaluate(model.Snapshot{Timestamp: now.Add(time.Second), CPU: model.CPU{Total: &v}})
	if len(m.History()) != 1 {
		t.Fatal("should stay active inside hysteresis")
	}
	v = 74
	m.Evaluate(model.Snapshot{Timestamp: now.Add(2 * time.Second), CPU: model.CPU{Total: &v}})
	if len(m.History()) != 2 || m.History()[1].State != "resolved" {
		t.Fatal("expected resolved")
	}
}
