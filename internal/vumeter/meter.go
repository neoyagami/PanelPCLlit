package vumeter

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"math/cmplx"
	"os/exec"
	"time"
)

type Config struct {
	Enabled    bool
	SourceKind string
	Target     string
	MinDB      float64
	MaxDB      float64
	FPS        int
}

type Resolver func(kind, target string) ([]string, error)

type Frame struct {
	Level float64
	Bands [4]float64
}

type Meter struct {
	updates chan Config
	resolve Resolver
	onFrame func(Frame)
	onError func(error)
}

func New(resolve Resolver, onFrame func(Frame), onError func(error)) *Meter {
	return &Meter{
		updates: make(chan Config, 1),
		resolve: resolve,
		onFrame: onFrame,
		onError: onError,
	}
}

func (m *Meter) Set(cfg Config) {
	select {
	case <-m.updates:
	default:
	}
	select {
	case m.updates <- cfg:
	default:
	}
}

func (m *Meter) Run(ctx context.Context) {
	var stop context.CancelFunc
	for {
		select {
		case <-ctx.Done():
			if stop != nil {
				stop()
			}
			return
		case cfg := <-m.updates:
			if stop != nil {
				stop()
				stop = nil
			}
			if !cfg.Enabled {
				m.onFrame(Frame{})
				m.onError(nil)
				continue
			}
			captureCtx, cancel := context.WithCancel(ctx)
			stop = cancel
			go m.captureLoop(captureCtx, cfg)
		}
	}
}

func (m *Meter) captureLoop(ctx context.Context, cfg Config) {
	for ctx.Err() == nil {
		err := m.captureOnce(ctx, cfg)
		if ctx.Err() != nil {
			return
		}
		m.onError(err)
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

func (m *Meter) captureOnce(ctx context.Context, cfg Config) error {
	sourceArgs, err := m.resolve(cfg.SourceKind, cfg.Target)
	if err != nil {
		return err
	}
	args := []string{
		"--raw",
		"--format=float32le",
		"--rate=48000",
		"--channels=1",
		"--latency-msec=20",
		"--process-time-msec=20",
		"--client-name=PanelPC-VU",
	}
	args = append(args, sourceArgs...)
	cmd := exec.CommandContext(ctx, "parec", args...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("create VU capture: %w", err)
	}
	var stderr limitedBuffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start parec: %w", err)
	}

	// 20 ms de float32 mono a 48 kHz. io.ReadFull evita interpretar muestras
	// partidas entre lecturas del pipe.
	buffer := make([]byte, 960*4)
	frameInterval := time.Second / time.Duration(clampFPS(cfg.FPS))
	lastEmit := time.Now()
	var smoothed, windowPeak Frame
	started := false
	for {
		_, readErr := io.ReadFull(stdout, buffer)
		if readErr != nil {
			waitErr := cmd.Wait()
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if waitErr == nil && errors.Is(readErr, io.EOF) {
				return errors.New("parec exited without producing data")
			}
			message := bytes.TrimSpace(stderr.Bytes())
			if len(message) > 0 {
				return fmt.Errorf("parec: %s", message)
			}
			if waitErr != nil {
				return fmt.Errorf("parec: %w", waitErr)
			}
			return fmt.Errorf("read VU audio: %w", readErr)
		}

		frame := analyzeFrame(buffer, cfg.MinDB, cfg.MaxDB)
		smoothed.Level = smooth(smoothed.Level, frame.Level)
		windowPeak.Level = math.Max(windowPeak.Level, smoothed.Level)
		for i := range smoothed.Bands {
			smoothed.Bands[i] = smooth(smoothed.Bands[i], frame.Bands[i])
			windowPeak.Bands[i] = math.Max(windowPeak.Bands[i], smoothed.Bands[i])
		}
		if time.Since(lastEmit) >= frameInterval {
			if !started {
				m.onError(nil)
				started = true
			}
			m.onFrame(windowPeak)
			windowPeak = Frame{}
			lastEmit = time.Now()
		}
	}
}

func analyzeFrame(data []byte, minDB, maxDB float64) Frame {
	return Frame{
		Level: normalizedPeak(data, minDB, maxDB),
		Bands: spectrumBands(data, minDB, maxDB),
	}
}

func smooth(previous, current float64) float64 {
	factor := 0.22
	if current > previous {
		factor = 0.72
	}
	return previous + (current-previous)*factor
}

func normalizedPeak(data []byte, minDB, maxDB float64) float64 {
	peak := 0.0
	for offset := 0; offset+4 <= len(data); offset += 4 {
		bits := binary.LittleEndian.Uint32(data[offset : offset+4])
		value := math.Abs(float64(math.Float32frombits(bits)))
		if value > peak {
			peak = value
		}
	}
	return normalizeAmplitude(peak, minDB, maxDB)
}

func spectrumBands(data []byte, minDB, maxDB float64) [4]float64 {
	const (
		fftSize    = 1024
		sampleRate = 48000
	)
	values := make([]complex128, fftSize)
	samples := len(data) / 4
	if samples > fftSize {
		samples = fftSize
	}
	for i := 0; i < samples; i++ {
		bits := binary.LittleEndian.Uint32(data[i*4 : i*4+4])
		window := 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(fftSize-1))
		values[i] = complex(float64(math.Float32frombits(bits))*window, 0)
	}
	fft(values)
	edges := [5]float64{60, 250, 1000, 4000, 20000}
	var amplitudes [4]float64
	for bin := 1; bin < fftSize/2; bin++ {
		frequency := float64(bin) * sampleRate / fftSize
		for band := range amplitudes {
			if frequency < edges[band] || frequency >= edges[band+1] {
				continue
			}
			// La ventana Hann tiene ganancia coherente cercana a 0.5; el factor
			// cuatro recupera aproximadamente la amplitud pico de un seno.
			amplitude := 4 * cmplx.Abs(values[bin]) / fftSize
			if amplitude > amplitudes[band] {
				amplitudes[band] = amplitude
			}
			break
		}
	}
	var result [4]float64
	for i, amplitude := range amplitudes {
		result[i] = normalizeAmplitude(amplitude, minDB, maxDB)
	}
	return result
}

func normalizeAmplitude(amplitude, minDB, maxDB float64) float64 {
	if maxDB <= minDB {
		minDB, maxDB = -48, 0
	}
	db := minDB
	if amplitude > 0.000001 {
		db = 20 * math.Log10(amplitude)
	}
	level := (db - minDB) / (maxDB - minDB)
	return math.Max(0, math.Min(1, level))
}

func fft(values []complex128) {
	n := len(values)
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			values[i], values[j] = values[j], values[i]
		}
	}
	for length := 2; length <= n; length <<= 1 {
		angle := -2 * math.Pi / float64(length)
		step := cmplx.Rect(1, angle)
		for start := 0; start < n; start += length {
			weight := complex(1.0, 0)
			for offset := 0; offset < length/2; offset++ {
				even := values[start+offset]
				odd := values[start+offset+length/2] * weight
				values[start+offset] = even + odd
				values[start+offset+length/2] = even - odd
				weight *= step
			}
		}
	}
}

func clampFPS(fps int) int {
	if fps < 5 {
		return 5
	}
	if fps > 30 {
		return 30
	}
	return fps
}

type limitedBuffer struct {
	data []byte
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	original := len(data)
	remaining := 4096 - len(b.data)
	if remaining > 0 {
		if len(data) > remaining {
			data = data[:remaining]
		}
		b.data = append(b.data, data...)
	}
	return original, nil
}

func (b *limitedBuffer) Bytes() []byte { return b.data }
