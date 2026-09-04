package server

import (
	"crypto/rand"
	"crypto/subtle"
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"

	"panelpc/internal/audio"
	"panelpc/internal/config"
	"panelpc/internal/desktopapps"
	"panelpc/internal/device"
	"panelpc/internal/engine"
)

//go:embed web/*
var webFiles embed.FS

type Server struct {
	configPath string
	configMu   sync.RWMutex
	config     config.Config
	device     *device.Manager
	engine     *engine.Engine
	audio      *audio.Controller
	token      string
}

func New(configPath string, cfg config.Config, dev *device.Manager, eng *engine.Engine, aud *audio.Controller) *Server {
	secret := make([]byte, 24)
	if _, err := rand.Read(secret); err != nil {
		panic(err)
	}
	s := &Server{
		configPath: configPath,
		config:     cfg,
		device:     dev,
		engine:     eng,
		audio:      aud,
		token:      base64.RawURLEncoding.EncodeToString(secret),
	}
	eng.SetProfileSwitcher(s.ActivateProfile)
	return s
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.index)
	mux.HandleFunc("GET /api/config", s.authorize(s.getConfig))
	mux.HandleFunc("PUT /api/config", s.authorize(s.putConfig))
	mux.HandleFunc("GET /api/status", s.authorize(s.getStatus))
	mux.HandleFunc("GET /api/audio/apps", s.authorize(s.getApps))
	mux.HandleFunc("GET /api/audio/devices", s.authorize(s.getDevices))
	mux.HandleFunc("GET /api/desktop/apps", s.authorize(s.getDesktopApps))
	mux.HandleFunc("POST /api/test", s.authorize(s.postTest))
	mux.HandleFunc("POST /api/obs/test", s.authorize(s.testOBS))
	mux.HandleFunc("GET /api/obs/inputs", s.authorize(s.getOBSInputs))
	mux.HandleFunc("GET /api/obs/filters", s.authorize(s.getOBSFilters))
	mux.HandleFunc("GET /api/v1/state", s.authorizeIntegration(s.getIntegrationState))
	mux.HandleFunc("POST /api/v1/actions", s.authorizeIntegration(s.postIntegrationAction))
	assets, _ := fs.Sub(webFiles, "web")
	mux.Handle("GET /assets/", http.StripPrefix("/assets/", noCache(http.FileServer(http.FS(assets)))))
	return recoverer(localOnly(mux))
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	data, err := webFiles.ReadFile("web/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	page := strings.ReplaceAll(string(data), "__PANELPC_TOKEN__", s.token)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; frame-ancestors 'none'")
	_, _ = io.WriteString(w, page)
}

func (s *Server) authorize(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-PanelPC-Token") != s.token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			allowedHTTP := "http://" + r.Host
			allowedHTTPS := "https://" + r.Host
			if origin != allowedHTTP && origin != allowedHTTPS {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) authorizeIntegration(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		parts := strings.Fields(r.Header.Get("Authorization"))
		s.configMu.RLock()
		expected := s.config.API.Token
		s.configMu.RUnlock()
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || subtle.ConstantTimeCompare([]byte(parts[1]), []byte(expected)) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="PanelPC API"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			allowedHTTP := "http://" + r.Host
			allowedHTTPS := "https://" + r.Host
			if origin != allowedHTTP && origin != allowedHTTPS {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

type integrationKnobState struct {
	Number  int    `json:"number"`
	Label   string `json:"label"`
	Value   int    `json:"value"`
	Percent int    `json:"percent"`
}

type integrationState struct {
	ActiveProfile string                  `json:"activeProfile"`
	Device        device.Status           `json:"device"`
	Knobs         [4]integrationKnobState `json:"knobs"`
	LastEvent     string                  `json:"lastEvent,omitempty"`
	LastError     string                  `json:"lastError,omitempty"`
	AudioError    string                  `json:"audioError,omitempty"`
	LightingMode  string                  `json:"lightingMode"`
	VULevel       float64                 `json:"vuLevel"`
	VUError       string                  `json:"vuError,omitempty"`
	Spectrum      [4]float64              `json:"spectrum"`
}

func (s *Server) getIntegrationState(w http.ResponseWriter, _ *http.Request) {
	engineStatus := s.engine.Status()
	s.configMu.RLock()
	cfg := s.config
	s.configMu.RUnlock()
	state := integrationState{
		ActiveProfile: cfg.ActiveProfile,
		Device:        s.device.Status(),
		LastEvent:     engineStatus.LastEvent,
		LastError:     engineStatus.LastError,
		AudioError:    s.audio.LastError(),
		LightingMode:  cfg.Lighting.Mode,
		VULevel:       engineStatus.VULevel,
		VUError:       engineStatus.VUError,
		Spectrum:      engineStatus.Spectrum,
	}
	for index, value := range engineStatus.Values {
		state.Knobs[index] = integrationKnobState{
			Number:  index + 1,
			Label:   cfg.Knobs[index].Label,
			Value:   value,
			Percent: (value*100 + 127) / 255,
		}
	}
	writeJSON(w, http.StatusOK, state)
}

type integrationAction struct {
	Knob  int    `json:"knob"`
	Kind  string `json:"kind"`
	Value int    `json:"value,omitempty"`
}

func (s *Server) postIntegrationAction(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "application/json is required", http.StatusUnsupportedMediaType)
		return
	}
	var action integrationAction
	decoder := json.NewDecoder(io.LimitReader(r.Body, 4096))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&action); err != nil {
		http.Error(w, "invalid action: "+err.Error(), http.StatusBadRequest)
		return
	}
	if action.Knob < 1 || action.Knob > 4 {
		http.Error(w, "knob must be between 1 and 4", http.StatusBadRequest)
		return
	}
	event := device.Event{Knob: action.Knob - 1}
	switch action.Kind {
	case "turn":
		if action.Value < 0 || action.Value > 255 {
			http.Error(w, "turn value must be between 0 and 255", http.StatusBadRequest)
			return
		}
		event.Kind = "turn"
		event.Value = action.Value
	case "click":
		event.Kind = "press"
		event.Value = 1
	default:
		http.Error(w, "kind must be turn or click", http.StatusBadRequest)
		return
	}
	s.engine.Inject(event)
	writeJSON(w, http.StatusAccepted, map[string]bool{"accepted": true})
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.Config())
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "application/json is required", http.StatusUnsupportedMediaType)
		return
	}
	var cfg config.Config
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		http.Error(w, "invalid configuration: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.UpdateConfig(cfg); err != nil {
		http.Error(w, "save configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) getStatus(w http.ResponseWriter, _ *http.Request) {
	s.configMu.RLock()
	activeProfile := s.config.ActiveProfile
	s.configMu.RUnlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"device":        s.device.Status(),
		"engine":        s.engine.Status(),
		"audioError":    s.audio.LastError(),
		"activeProfile": activeProfile,
	})
}

// Config returns a detached snapshot for native frontends and integrations.
func (s *Server) Config() config.Config {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	return s.config.Clone()
}

// UpdateConfig persists and applies a complete configuration atomically from
// the perspective of readers. It is shared by the web and native frontends.
func (s *Server) UpdateConfig(cfg config.Config) error {
	cfg.Normalize()
	if err := config.Save(s.configPath, cfg); err != nil {
		return err
	}
	s.configMu.Lock()
	s.config = cfg.Clone()
	s.configMu.Unlock()
	s.engine.Configure(cfg)
	return nil
}

// ActivateProfile persists and applies a saved profile without using HTTP.
func (s *Server) ActivateProfile(name string) error {
	s.configMu.Lock()
	cfg := s.config.Clone()
	if err := cfg.ActivateProfile(name); err != nil {
		s.configMu.Unlock()
		return err
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		s.configMu.Unlock()
		return fmt.Errorf("save active profile: %w", err)
	}
	s.config = cfg
	s.configMu.Unlock()
	s.engine.Configure(cfg)
	return nil
}

func (s *Server) getApps(w http.ResponseWriter, _ *http.Request) {
	apps, err := s.audio.Apps(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, apps)
}

func (s *Server) getDevices(w http.ResponseWriter, _ *http.Request) {
	devices, err := s.audio.Devices(true)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, devices)
}

func (s *Server) getDesktopApps(w http.ResponseWriter, _ *http.Request) {
	applications, err := desktopapps.Discover()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, applications)
}

func (s *Server) postTest(w http.ResponseWriter, r *http.Request) {
	var event device.Event
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if event.Knob < 0 || event.Knob > 3 || (event.Kind != "turn" && event.Kind != "press") || event.Value < 0 || event.Value > 255 {
		http.Error(w, "invalid test event", http.StatusBadRequest)
		return
	}
	s.engine.Inject(event)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) testOBS(w http.ResponseWriter, _ *http.Request) {
	if err := s.engine.TestOBS(); err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) getOBSInputs(w http.ResponseWriter, _ *http.Request) {
	inputs, err := s.engine.OBSInputs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, inputs)
}

func (s *Server) getOBSFilters(w http.ResponseWriter, r *http.Request) {
	source := strings.TrimSpace(r.URL.Query().Get("source"))
	if source == "" {
		http.Error(w, "source is required", http.StatusBadRequest)
		return
	}
	filters, err := s.engine.OBSFilters(source)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	writeJSON(w, http.StatusOK, filters)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func noCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func localOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if parsedHost, _, err := net.SplitHostPort(r.Host); err == nil {
			host = parsedHost
		}
		host = strings.Trim(host, "[]")
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			http.Error(w, "host not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("HTTP panic: %v", recovered)
				http.Error(w, fmt.Sprint(recovered), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
