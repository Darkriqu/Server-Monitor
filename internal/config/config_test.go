package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	t.Setenv("SERVER_MONITOR_TOKEN", "test-token")
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(path, []byte("listen = \"127.0.0.1:9999\"\ninterval = \"500ms\"\nhistory = \"2h\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Interval != 500*time.Millisecond || c.History != 2*time.Hour {
		t.Fatalf("unexpected config: %#v", c)
	}
}
