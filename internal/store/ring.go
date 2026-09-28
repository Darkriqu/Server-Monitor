package store

import (
	"github.com/Darkriqu/Server-Monitor/internal/model"
	"sync"
	"time"
)

type Ring struct {
	mu         sync.RWMutex
	data       []model.Snapshot
	head, size int
}

func NewRing(capacity int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	return &Ring{data: make([]model.Snapshot, capacity)}
}
func (r *Ring) Add(s model.Snapshot) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.data[r.head] = s
	r.head = (r.head + 1) % len(r.data)
	if r.size < len(r.data) {
		r.size++
	}
}
func (r *Ring) Latest() (model.Snapshot, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.size == 0 {
		return model.Snapshot{}, false
	}
	i := (r.head - 1 + len(r.data)) % len(r.data)
	return r.data[i], true
}
func (r *Ring) Query(from, to time.Time, step time.Duration) []model.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]model.Snapshot, 0, r.size)
	start := (r.head - r.size + len(r.data)) % len(r.data)
	var last time.Time
	for n := 0; n < r.size; n++ {
		s := r.data[(start+n)%len(r.data)]
		if !from.IsZero() && s.Timestamp.Before(from) {
			continue
		}
		if !to.IsZero() && s.Timestamp.After(to) {
			continue
		}
		if step > 0 && !last.IsZero() && s.Timestamp.Sub(last) < step {
			continue
		}
		out = append(out, s)
		last = s.Timestamp
	}
	return out
}
func (r *Ring) Len() int { r.mu.RLock(); defer r.mu.RUnlock(); return r.size }
