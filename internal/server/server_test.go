package server

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"panelpc/internal/audio"
	"panelpc/internal/config"
	"panelpc/internal/device"
	"panelpc/internal/engine"
)

func testServer(t *testing.T) (*Server, http.Handler) {
	t.Helper()
	cfg := config.Default()
	dev := device.NewManager()
	aud := audio.New()
	eng := engine.New(dev, aud, cfg)
	srv := New(filepath.Join(t.TempDir(), "config.json"), cfg, dev, eng, aud)
	return srv, srv.Handler()
}

func TestAPIRequiresToken(t *testing.T) {
	_, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Host = "127.0.0.1:8765"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
}

func TestIndexDoesNotCacheAndContainsToken(t *testing.T) {
	srv, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "127.0.0.1:8765"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), srv.token) {
		t.Fatalf("índice inválido: status %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q", got)
	}
}

func TestAPIDeniesForeignOrigin(t *testing.T) {
	srv, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Host = "127.0.0.1:8765"
	req.Header.Set("X-PanelPC-Token", srv.token)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestServerDeniesForeignHost(t *testing.T) {
	_, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "panelpc.attacker.example"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}
