package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Darkriqu/Server-Monitor/internal/model"
	_ "modernc.org/sqlite"
	"sync"
	"time"
)

type SQLite struct {
	db      *sql.DB
	mu      sync.Mutex
	pending []model.Snapshot
	batch   int
}

func OpenSQLite(path string, batch int) (*SQLite, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	for _, q := range []string{"PRAGMA journal_mode=WAL;", "PRAGMA synchronous=NORMAL;", "CREATE TABLE IF NOT EXISTS metrics(ts INTEGER PRIMARY KEY, bucket INTEGER NOT NULL DEFAULT 1, data BLOB NOT NULL);", "CREATE INDEX IF NOT EXISTS idx_metrics_bucket_ts ON metrics(bucket,ts);"} {
		if _, err = db.Exec(q); err != nil {
			db.Close()
			return nil, err
		}
	}
	if batch < 1 {
		batch = 60
	}
	return &SQLite{db: db, batch: batch, pending: make([]model.Snapshot, 0, batch)}, nil
}
func (s *SQLite) Queue(v model.Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending = append(s.pending, v)
	if len(s.pending) >= s.batch {
		return s.flushLocked()
	}
	return nil
}
func (s *SQLite) Flush() error { s.mu.Lock(); defer s.mu.Unlock(); return s.flushLocked() }
func (s *SQLite) flushLocked() error {
	if len(s.pending) == 0 {
		return nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	st, err := tx.Prepare("INSERT OR REPLACE INTO metrics(ts,bucket,data) VALUES(?,?,?)")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer st.Close()
	for _, v := range s.pending {
		b, _ := json.Marshal(v)
		if _, err = st.Exec(v.Timestamp.Unix(), 1, b); err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	s.pending = s.pending[:0]
	return nil
}
func (s *SQLite) Load(ctx context.Context, from, to time.Time, step time.Duration) ([]model.Snapshot, error) {
	q := "SELECT data FROM metrics WHERE ts>=? AND ts<=? ORDER BY ts"
	rows, err := s.db.QueryContext(ctx, q, from.Unix(), to.Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Snapshot
	var last time.Time
	for rows.Next() {
		var b []byte
		if err := rows.Scan(&b); err != nil {
			return nil, err
		}
		var v model.Snapshot
		if err := json.Unmarshal(b, &v); err != nil {
			return nil, err
		}
		if step > 0 && !last.IsZero() && v.Timestamp.Sub(last) < step {
			continue
		}
		out = append(out, v)
		last = v.Timestamp
	}
	return out, rows.Err()
}
func (s *SQLite) Compact(ctx context.Context) error {
	now := time.Now().UTC()
	h1 := now.Add(-time.Hour).Unix()
	d7 := now.Add(-7 * 24 * time.Hour).Unix()
	d90 := now.Add(-90 * 24 * time.Hour).Unix()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	queries := []struct {
		q string
		a []any
	}{
		{"DELETE FROM metrics WHERE ts < ? AND ts >= ? AND ts NOT IN (SELECT MIN(ts) FROM metrics WHERE ts < ? AND ts >= ? GROUP BY CAST(ts/60 AS INTEGER))", []any{h1, d7, h1, d7}},
		{"UPDATE metrics SET bucket=60 WHERE ts < ? AND ts >= ?", []any{h1, d7}},
		{"DELETE FROM metrics WHERE ts < ? AND ts >= ? AND ts NOT IN (SELECT MIN(ts) FROM metrics WHERE ts < ? AND ts >= ? GROUP BY CAST(ts/300 AS INTEGER))", []any{d7, d90, d7, d90}},
		{"UPDATE metrics SET bucket=300 WHERE ts < ? AND ts >= ?", []any{d7, d90}},
	}
	for _, x := range queries {
		if _, err = tx.ExecContext(ctx, x.q, x.a...); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("compact: %w", err)
		}
	}
	return tx.Commit()
}
func (s *SQLite) Cleanup(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM metrics WHERE ts < ?", time.Now().Add(-90*24*time.Hour).Unix())
	return err
}
func (s *SQLite) Close() error { _ = s.Flush(); return s.db.Close() }
