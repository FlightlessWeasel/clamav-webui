package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/FlightlessWeasel/clamav-webui/internal/config"
	"github.com/FlightlessWeasel/clamav-webui/internal/db"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	d, err := db.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	cfg := config.Defaults()
	cfg.ConfigDir = t.TempDir()
	s, err := New(cfg, d, "test-1.0")
	if err != nil {
		t.Fatalf("api.New: %v", err)
	}
	return s
}

func TestHealth(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var body map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["status"] != "ok" {
		t.Errorf("body = %v", body)
	}
}

func TestVersion(t *testing.T) {
	s := newTestServer(t)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/version", nil))

	var body map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["version"] != "test-1.0" {
		t.Errorf("version = %q", body["version"])
	}
}

func TestSPAFallback(t *testing.T) {
	s := newTestServer(t)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/dashboard", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("SPA route status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct[:9] != "text/html" {
		t.Errorf("Content-Type = %q", ct)
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/missing.js", nil))
	if rec.Code != http.StatusNotFound {
		t.Errorf("unknown asset status = %d, want 404", rec.Code)
	}
}
