package vumeter

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestNormalizedPeak(t *testing.T) {
	data := make([]byte, 8)
	binary.LittleEndian.PutUint32(data[0:4], math.Float32bits(0.01)) // -40 dB
	binary.LittleEndian.PutUint32(data[4:8], math.Float32bits(-0.1)) // -20 dB
	got := normalizedPeak(data, -40, 0)
	if math.Abs(got-0.5) > 0.001 {
		t.Fatalf("level = %f, expected 0.5", got)
	}
}

func TestNormalizedPeakClamps(t *testing.T) {
	silence := make([]byte, 4)
	if got := normalizedPeak(silence, -48, 0); got != 0 {
		t.Fatalf("silence = %f", got)
	}
	loud := make([]byte, 4)
	binary.LittleEndian.PutUint32(loud, math.Float32bits(2))
	if got := normalizedPeak(loud, -48, 0); got != 1 {
		t.Fatalf("loud level = %f", got)
	}
}

func TestClampFPS(t *testing.T) {
	if clampFPS(1) != 5 || clampFPS(20) != 20 || clampFPS(100) != 30 {
		t.Fatal("clampFPS did not enforce 5..30")
	}
}

func TestSpectrumBands(t *testing.T) {
	tests := []struct {
		frequency float64
		band      int
	}{
		{frequency: 120, band: 0},
		{frequency: 500, band: 1},
		{frequency: 2000, band: 2},
		{frequency: 8000, band: 3},
	}
	for _, test := range tests {
		data := sineData(test.frequency, 0.5, 1024)
		bands := spectrumBands(data, -60, 0)
		for index, value := range bands {
			if index != test.band && value >= bands[test.band] {
				t.Fatalf("%.0f Hz sine: bands %#v, maximum outside %d", test.frequency, bands, test.band)
			}
		}
	}
}

func sineData(frequency, amplitude float64, samples int) []byte {
	data := make([]byte, samples*4)
	for i := 0; i < samples; i++ {
		value := float32(amplitude * math.Sin(2*math.Pi*frequency*float64(i)/48000))
		binary.LittleEndian.PutUint32(data[i*4:i*4+4], math.Float32bits(value))
	}
	return data
}
