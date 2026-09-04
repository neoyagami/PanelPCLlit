package server

import (
	"encoding/json"
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
		t.Fatalf("invalid index: status %d", rec.Code)
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

func TestIntegrationAPIRequiresBearerToken(t *testing.T) {
	_, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	req.Host = "127.0.0.1:8765"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got == "" {
		t.Fatal("WWW-Authenticate header is missing")
	}
}

func TestIntegrationAPIReportsKnobState(t *testing.T) {
	srv, handler := testServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/state", nil)
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Authorization", "Bearer "+srv.config.API.Token)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var state integrationState
	if err := json.NewDecoder(rec.Body).Decode(&state); err != nil {
		t.Fatal(err)
	}
	if state.ActiveProfile != "Main" || state.Knobs[0].Number != 1 || state.Knobs[0].Label != "Knob 1" {
		t.Fatalf("unexpected state: %#v", state)
	}
}

func TestIntegrationAPILaunchesConfiguredKnobEvent(t *testing.T) {
	srv, handler := testServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/actions", strings.NewReader(`{"knob":2,"kind":"click"}`))
	req.Host = "127.0.0.1:8765"
	req.Header.Set("Authorization", "Bearer "+srv.config.API.Token)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	event := <-srv.device.Events()
	if event.Knob != 1 || event.Kind != "press" || event.Value != 1 {
		t.Fatalf("unexpected event: %#v", event)
	}
}
