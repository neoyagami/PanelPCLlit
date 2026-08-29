package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type App struct {
	ID   uint32 `json:"id"`
	Name string `json:"name"`
}

type Device struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Kind    string `json:"kind"`
	State   string `json:"state,omitempty"`
	Monitor string `json:"-"`
}

type Controller struct {
	mu        sync.Mutex
	apps      []App
	appsAt    time.Time
	devices   []Device
	devicesAt time.Time
	lastError string
}

type pactlStream struct {
	Index      uint32            `json:"index"`
	Properties map[string]string `json:"properties"`
}

type pactlDevice struct {
	Name          string `json:"name"`
	Description   string `json:"description"`
	State         string `json:"state"`
	MonitorSource string `json:"monitor_source"`
}

func New() *Controller { return &Controller{} }

func (c *Controller) LastError() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastError
}

func (c *Controller) setError(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err == nil {
		c.lastError = ""
	} else {
		c.lastError = err.Error()
	}
}

func (c *Controller) SetVolume(kind, target string, percent float64) error {
	if percent < 0 {
		percent = 0
	}
	if percent > 150 {
		percent = 150
	}
	var err error
	switch kind {
	case "output":
		err = command("wpctl", "set-volume", "@DEFAULT_AUDIO_SINK@", fmt.Sprintf("%.3f", percent/100))
	case "input":
		err = command("wpctl", "set-volume", "@DEFAULT_AUDIO_SOURCE@", fmt.Sprintf("%.3f", percent/100))
	case "output_device":
		err = command("pactl", "set-sink-volume", target, fmt.Sprintf("%.1f%%", percent))
	case "input_device":
		err = command("pactl", "set-source-volume", target, fmt.Sprintf("%.1f%%", percent))
	case "app":
		var apps []App
		apps, err = c.Apps(false)
		if err == nil {
			matches := matchApps(apps, target)
			if len(matches) == 0 {
				err = fmt.Errorf("no hay canal de audio que coincida con %q", target)
			} else {
				for _, app := range matches {
					if callErr := command("pactl", "set-sink-input-volume", strconv.FormatUint(uint64(app.ID), 10), fmt.Sprintf("%.1f%%", percent)); callErr != nil {
						err = callErr
						break
					}
				}
			}
		}
	default:
		err = fmt.Errorf("tipo de volumen desconocido: %s", kind)
	}
	c.setError(err)
	return err
}

func (c *Controller) ToggleMute(kind, target string) error {
	var err error
	switch kind {
	case "output":
		err = command("wpctl", "set-mute", "@DEFAULT_AUDIO_SINK@", "toggle")
	case "input":
		err = command("wpctl", "set-mute", "@DEFAULT_AUDIO_SOURCE@", "toggle")
	case "output_device":
		err = command("pactl", "set-sink-mute", target, "toggle")
	case "input_device":
		err = command("pactl", "set-source-mute", target, "toggle")
	case "app":
		var apps []App
		apps, err = c.Apps(false)
		if err == nil {
			matches := matchApps(apps, target)
			if len(matches) == 0 {
				err = fmt.Errorf("no hay canal de audio que coincida con %q", target)
			} else {
				for _, app := range matches {
					if callErr := command("pactl", "set-sink-input-mute", strconv.FormatUint(uint64(app.ID), 10), "toggle"); callErr != nil {
						err = callErr
						break
					}
				}
			}
		}
	default:
		err = fmt.Errorf("no se puede silenciar %s", kind)
	}
	c.setError(err)
	return err
}

func (c *Controller) Apps(force bool) ([]App, error) {
	c.mu.Lock()
	if !force && time.Since(c.appsAt) < time.Second {
		result := append([]App(nil), c.apps...)
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pactl", "--format=json", "list", "sink-inputs").Output()
	if err != nil {
		return nil, fmt.Errorf("listar aplicaciones con pactl: %w", err)
	}
	var streams []pactlStream
	if err := json.Unmarshal(out, &streams); err != nil {
		return nil, fmt.Errorf("respuesta inválida de pactl: %w", err)
	}
	apps := make([]App, 0, len(streams))
	for _, stream := range streams {
		name := firstNonEmpty(
			stream.Properties["application.name"],
			stream.Properties["application.process.binary"],
			stream.Properties["media.name"],
		)
		if name != "" {
			apps = append(apps, App{ID: stream.Index, Name: name})
		}
	}
	sort.Slice(apps, func(i, j int) bool { return strings.ToLower(apps[i].Name) < strings.ToLower(apps[j].Name) })
	c.mu.Lock()
	c.apps, c.appsAt = apps, time.Now()
	c.mu.Unlock()
	return append([]App(nil), apps...), nil
}

func (c *Controller) Devices(force bool) ([]Device, error) {
	c.mu.Lock()
	if !force && time.Since(c.devicesAt) < 5*time.Second {
		result := append([]Device(nil), c.devices...)
		c.mu.Unlock()
		return result, nil
	}
	c.mu.Unlock()

	outputs, err := listDevices("output", "sinks")
	if err != nil {
		c.setError(err)
		return nil, err
	}
	inputs, err := listDevices("input", "sources")
	if err != nil {
		c.setError(err)
		return nil, err
	}
	devices := append(outputs, inputs...)
	sort.Slice(devices, func(i, j int) bool {
		if devices[i].Kind != devices[j].Kind {
			return devices[i].Kind < devices[j].Kind
		}
		return strings.ToLower(devices[i].Name) < strings.ToLower(devices[j].Name)
	})
	c.mu.Lock()
	c.devices, c.devicesAt = devices, time.Now()
	c.lastError = ""
	c.mu.Unlock()
	return append([]Device(nil), devices...), nil
}

// VUCaptureArgs resuelve una selección estable de la interfaz a los argumentos
// de parec. PipeWire expone esta interfaz por su servidor compatible con PulseAudio.
func (c *Controller) VUCaptureArgs(kind, target string) ([]string, error) {
	switch kind {
	case "output":
		return []string{"--device=@DEFAULT_MONITOR@"}, nil
	case "input":
		return []string{"--device=@DEFAULT_SOURCE@"}, nil
	case "app":
		apps, err := c.Apps(false)
		if err != nil {
			return nil, err
		}
		matches := matchApps(apps, target)
		if len(matches) == 0 {
			return nil, fmt.Errorf("no hay canal de audio que coincida con %q", target)
		}
		return []string{"--monitor-stream=" + strconv.FormatUint(uint64(matches[0].ID), 10)}, nil
	case "output_device", "input_device":
		devices, err := c.Devices(false)
		if err != nil {
			return nil, err
		}
		wantedKind := strings.TrimSuffix(kind, "_device")
		for _, device := range devices {
			if device.Kind != wantedKind || device.ID != target {
				continue
			}
			capture := device.ID
			if wantedKind == "output" {
				capture = device.Monitor
				if capture == "" {
					capture = device.ID + ".monitor"
				}
			}
			return []string{"--device=" + capture}, nil
		}
		return nil, fmt.Errorf("no existe el dispositivo de %s %q", wantedKind, target)
	default:
		return nil, fmt.Errorf("fuente VU desconocida: %s", kind)
	}
}

func listDevices(kind, object string) ([]Device, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "pactl", "--format=json", "list", object).Output()
	if err != nil {
		return nil, fmt.Errorf("listar %s con pactl: %w", object, err)
	}
	return parseDevices(out, kind)
}

func parseDevices(data []byte, kind string) ([]Device, error) {
	var raw []pactlDevice
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("respuesta inválida de pactl: %w", err)
	}
	devices := make([]Device, 0, len(raw))
	for _, item := range raw {
		if item.Name == "" || (kind == "input" && item.MonitorSource != "") {
			continue
		}
		name := item.Description
		if name == "" {
			name = item.Name
		}
		devices = append(devices, Device{ID: item.Name, Name: name, Kind: kind, State: strings.ToLower(item.State), Monitor: item.MonitorSource})
	}
	return devices, nil
}

func matchApps(apps []App, target string) []App {
	needle := strings.ToLower(strings.TrimSpace(target))
	if needle == "" {
		return nil
	}
	var matches []App
	for _, app := range apps {
		if strings.Contains(strings.ToLower(app.Name), needle) {
			matches = append(matches, app)
		}
	}
	return matches
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func command(name string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		if ctx.Err() != nil {
			return fmt.Errorf("%s excedió 1.5 s", name)
		}
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("%s: %s", name, message)
	}
	return nil
}
