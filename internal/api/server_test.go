package api

import (
	"github.com/Darkriqu/Server-Monitor/internal/alerts"
	"github.com/Darkriqu/Server-Monitor/internal/config"
	"github.com/Darkriqu/Server-Monitor/internal/model"
	"github.com/Darkriqu/Server-Monitor/internal/store"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestHealthAndAuth(t *testing.T) {
	root := fstest.MapFS{"index.html": &fstest.MapFile{Data: []byte("ok")}}
	sub, _ := fs.Sub(root, ".")
	c := config.Default()
	c.Auth.Token = "secret"
	s := New(model.SystemInfo{}, store.NewRing(2), nil, alerts.New(c), "secret", sub)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	if r, e := http.Get(ts.URL + "/healthz"); e != nil || r.StatusCode != 200 {
		t.Fatalf("health failed")
	}
	req, _ := http.NewRequest("GET", ts.URL+"/api/v1/system", nil)
	req.Header.Set("Authorization", "Bearer secret")
	r, e := http.DefaultClient.Do(req)
	if e != nil || r.StatusCode != 200 {
		t.Fatalf("api failed: %v", e)
	}
}
