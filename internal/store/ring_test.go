package store

import (
	"github.com/Darkriqu/Server-Monitor/internal/model"
	"testing"
	"time"
)

func TestRingWrap(t *testing.T) {
	r := NewRing(2)
	now := time.Now()
	r.Add(model.Snapshot{Timestamp: now})
	r.Add(model.Snapshot{Timestamp: now.Add(time.Second)})
	r.Add(model.Snapshot{Timestamp: now.Add(2 * time.Second)})
	got := r.Query(time.Time{}, time.Time{}, 0)
	if len(got) != 2 || !got[0].Timestamp.Equal(now.Add(time.Second)) {
		t.Fatalf("unexpected ring contents: %#v", got)
	}
}
