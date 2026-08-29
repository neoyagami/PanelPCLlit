package server

import (
	"crypto/rand"
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
	eng.SetProfileSwitcher(s.switchProfile)
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
	mux.HandleFunc("POST /api/test", s.authorize(s.postTest))
	mux.HandleFunc("POST /api/obs/test", s.authorize(s.testOBS))
	mux.HandleFunc("GET /api/obs/inputs", s.authorize(s.getOBSInputs))
	mux.HandleFunc("GET /api/obs/filters", s.authorize(s.getOBSFilters))
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
			http.Error(w, "no autorizado", http.StatusUnauthorized)
			return
		}
		if origin := r.Header.Get("Origin"); origin != "" {
			allowedHTTP := "http://" + r.Host
			allowedHTTPS := "https://" + r.Host
			if origin != allowedHTTP && origin != allowedHTTPS {
				http.Error(w, "origen no permitido", http.StatusForbidden)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) getConfig(w http.ResponseWriter, _ *http.Request) {
	s.configMu.RLock()
	defer s.configMu.RUnlock()
	writeJSON(w, http.StatusOK, s.config)
}

func (s *Server) putConfig(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		http.Error(w, "se requiere application/json", http.StatusUnsupportedMediaType)
		return
	}
	var cfg config.Config
	decoder := json.NewDecoder(io.LimitReader(r.Body, 64*1024))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		http.Error(w, "configuración inválida: "+err.Error(), http.StatusBadRequest)
		return
	}
	cfg.Normalize()
	if err := config.Save(s.configPath, cfg); err != nil {
		http.Error(w, "guardar configuración: "+err.Error(), http.StatusInternalServerError)
		return
	}
	s.configMu.Lock()
	s.config = cfg
	s.configMu.Unlock()
	s.engine.Configure(cfg)
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

func (s *Server) switchProfile(name string) error {
	s.configMu.Lock()
	cfg := s.config
	if err := cfg.ActivateProfile(name); err != nil {
		s.configMu.Unlock()
		return err
	}
	if err := config.Save(s.configPath, cfg); err != nil {
		s.configMu.Unlock()
		return fmt.Errorf("guardar perfil activo: %w", err)
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

func (s *Server) postTest(w http.ResponseWriter, r *http.Request) {
	var event device.Event
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&event); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if event.Knob < 0 || event.Knob > 3 || (event.Kind != "turn" && event.Kind != "press") || event.Value < 0 || event.Value > 255 {
		http.Error(w, "evento de prueba inválido", http.StatusBadRequest)
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
		http.Error(w, "falta source", http.StatusBadRequest)
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
			http.Error(w, "host no permitido", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("panic HTTP: %v", recovered)
				http.Error(w, fmt.Sprint(recovered), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
