package device

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Event struct {
	Kind    string `json:"kind"`
	Knob    int    `json:"knob"`
	Value   int    `json:"value"`
	Initial bool   `json:"initial,omitempty"`
}

type Status struct {
	Connected bool   `json:"connected"`
	Device    string `json:"device,omitempty"`
	Error     string `json:"error,omitempty"`
}

type Lighting struct {
	GlobalBrightness int
	Knobs            [4]KnobLight
}

type KnobLight struct {
	Color      string
	TrackValue bool
}

type Manager struct {
	events   chan Event
	lighting chan Lighting
	mu       sync.RWMutex
	status   Status
	desired  Lighting
}

const productMini = "mini"

var supported = map[[2]uint16]string{
	{0x0483, 0xa3c4}: productMini,
	{0x0483, 0xa3c5}: "pro",
	{0x04d8, 0xeb52}: "rgb",
}

func NewManager() *Manager {
	return &Manager{events: make(chan Event, 32), lighting: make(chan Lighting, 1)}
}

func (m *Manager) Events() <-chan Event { return m.events }

// Inject alimenta el mismo canal acotado que el hardware. Se usa para probar
// una configuración desde la interfaz sin requerir que el PCPanel esté conectado.
func (m *Manager) Inject(event Event) { m.emit(event) }

// SetLighting conserva sólo la configuración RGB más nueva. El canal de salida
// tiene capacidad uno, igual que la cola de acciones, para impedir backlog USB.
func (m *Manager) SetLighting(lighting Lighting) {
	m.mu.Lock()
	m.desired = lighting
	m.mu.Unlock()
	select {
	case <-m.lighting:
	default:
	}
	select {
	case m.lighting <- lighting:
	default:
	}
}

func (m *Manager) Status() Status {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.status
}

func (m *Manager) setStatus(s Status) {
	m.mu.Lock()
	m.status = s
	m.mu.Unlock()
}

func (m *Manager) emit(event Event) {
	select {
	case m.events <- event:
	default:
		// La entrada física jamás puede bloquear al lector HID. Si la cola está
		// llena conservamos los eventos futuros, que llevan el valor absoluto.
	}
}

func (m *Manager) Run(ctx context.Context) {
	for ctx.Err() == nil {
		path, name, product, err := findDevice()
		if err != nil {
			m.setStatus(Status{Error: err.Error()})
			if !sleepContext(ctx, 2*time.Second) {
				return
			}
			continue
		}
		if path == "" {
			m.setStatus(Status{})
			if !sleepContext(ctx, 2*time.Second) {
				return
			}
			continue
		}

		fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if err != nil {
			m.setStatus(Status{Device: name, Error: fmt.Sprintf("%s: %v", path, err)})
			if !sleepContext(ctx, 2*time.Second) {
				return
			}
			continue
		}
		m.setStatus(Status{Connected: true, Device: name})
		if err := writeReport(ctx, fd, []byte{1}); err != nil {
			syscall.Close(fd)
			m.setStatus(Status{Device: name, Error: "inicializar HID: " + err.Error()})
			sleepContext(ctx, time.Second)
			continue
		}
		if product == productMini {
			m.mu.RLock()
			initialLighting := m.desired
			m.mu.RUnlock()
			if err := writeReport(ctx, fd, BuildMiniLightingReport(initialLighting)); err != nil {
				syscall.Close(fd)
				m.setStatus(Status{Device: name, Error: "configurar RGB: " + err.Error()})
				sleepContext(ctx, time.Second)
				continue
			}
		}
		err = m.readLoop(ctx, fd, product)
		syscall.Close(fd)
		if ctx.Err() != nil {
			return
		}
		m.setStatus(Status{Device: name, Error: err.Error()})
		sleepContext(ctx, time.Second)
	}
}

func (m *Manager) readLoop(ctx context.Context, fd int, product string) error {
	buf := make([]byte, 64)
	connectedAt := time.Now()
	var last [4]int
	var seen [4]bool
	for ctx.Err() == nil {
		n, err := syscall.Read(fd, buf)
		if err != nil {
			if errors.Is(err, syscall.EAGAIN) || errors.Is(err, syscall.EWOULDBLOCK) {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case lighting := <-m.lighting:
					if product == productMini {
						if err := writeReport(ctx, fd, BuildMiniLightingReport(lighting)); err != nil {
							return fmt.Errorf("escribir RGB: %w", err)
						}
					}
				case <-time.After(10 * time.Millisecond):
				}
				continue
			}
			return err
		}
		if n == 0 {
			return io.EOF
		}
		event, ok := ParseReport(buf[:n])
		if !ok {
			continue
		}
		// El firmware anuncia posiciones al conectar. Se usan como línea base y
		// no se ejecutan acciones durante una breve ventana de inicialización.
		if time.Since(connectedAt) < 400*time.Millisecond {
			if event.Kind == "turn" && event.Knob < len(last) {
				last[event.Knob], seen[event.Knob] = event.Value, true
				event.Initial = true
				m.emit(event)
			}
			continue
		}
		if event.Kind == "turn" {
			if event.Knob >= len(last) {
				continue
			}
			if seen[event.Knob] && last[event.Knob] == event.Value {
				continue
			}
			last[event.Knob], seen[event.Knob] = event.Value, true
		}
		m.emit(event)
		select {
		case lighting := <-m.lighting:
			if product == productMini {
				if err := writeReport(ctx, fd, BuildMiniLightingReport(lighting)); err != nil {
					return fmt.Errorf("escribir RGB: %w", err)
				}
			}
		default:
		}
	}
	return ctx.Err()
}

func ParseReport(data []byte) (Event, bool) {
	if len(data) < 3 {
		return Event{}, false
	}
	knob := int(data[1])
	if knob < 0 || knob > 3 {
		return Event{}, false
	}
	switch data[0] {
	case 1:
		return Event{Kind: "turn", Knob: knob, Value: int(data[2])}, true
	case 2:
		return Event{Kind: "press", Knob: knob, Value: int(data[2])}, true
	default:
		return Event{}, false
	}
}

func findDevice() (path, name, product string, err error) {
	entries, err := filepath.Glob("/sys/class/hidraw/hidraw*")
	if err != nil {
		return "", "", "", err
	}
	for _, entry := range entries {
		props, readErr := readUevent(filepath.Join(entry, "device", "uevent"))
		if readErr != nil {
			continue
		}
		parts := strings.Split(props["HID_ID"], ":")
		if len(parts) != 3 {
			continue
		}
		vid, errVID := strconv.ParseUint(parts[1], 16, 16)
		pid, errPID := strconv.ParseUint(parts[2], 16, 16)
		product, ok := supported[[2]uint16{uint16(vid), uint16(pid)}]
		if errVID != nil || errPID != nil || !ok {
			continue
		}
		return filepath.Join("/dev", filepath.Base(entry)), props["HID_NAME"], product, nil
	}
	return "", "", "", nil
}

func BuildMiniLightingReport(lighting Lighting) []byte {
	report := make([]byte, 64)
	report[0], report[1] = 6, 2 // Mini, iluminación custom de knobs
	brightness := lighting.GlobalBrightness
	if brightness < 0 {
		brightness = 0
	}
	if brightness > 100 {
		brightness = 100
	}
	for i, light := range lighting.Knobs {
		offset := 2 + i*7
		r, g, b := parseColor(light.Color)
		r = r * brightness / 100
		g = g * brightness / 100
		b = b * brightness / 100
		if light.TrackValue {
			report[offset] = 2 // gradiente de volumen: negro -> color
			report[offset+4], report[offset+5], report[offset+6] = byte(r), byte(g), byte(b)
		} else {
			report[offset] = 1 // color estático
			report[offset+1], report[offset+2], report[offset+3] = byte(r), byte(g), byte(b)
		}
	}
	return report
}

func parseColor(color string) (int, int, int) {
	if len(color) != 7 || color[0] != '#' {
		return 0, 0, 0
	}
	r, errR := strconv.ParseUint(color[1:3], 16, 8)
	g, errG := strconv.ParseUint(color[3:5], 16, 8)
	b, errB := strconv.ParseUint(color[5:7], 16, 8)
	if errR != nil || errG != nil || errB != nil {
		return 0, 0, 0
	}
	return int(r), int(g), int(b)
}

func writeReport(ctx context.Context, fd int, data []byte) error {
	report := make([]byte, 64)
	copy(report, data)
	deadline := time.Now().Add(150 * time.Millisecond)
	for {
		n, err := syscall.Write(fd, report)
		if err == nil {
			if n != len(report) {
				return fmt.Errorf("escritura HID parcial: %d de %d bytes", n, len(report))
			}
			return nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, syscall.EWOULDBLOCK) {
			return err
		}
		if time.Now().After(deadline) {
			return errors.New("timeout de escritura HID")
		}
		if !sleepContext(ctx, 5*time.Millisecond) {
			return ctx.Err()
		}
	}
}

func readUevent(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if ok {
			result[key] = value
		}
	}
	return result, scanner.Err()
}

func sleepContext(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
