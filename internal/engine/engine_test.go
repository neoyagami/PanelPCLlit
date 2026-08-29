package engine

import (
	"testing"

	"panelpc/internal/config"
)

func TestVULightingSegments(t *testing.T) {
	cfg := config.VU{MinColor: "#00ff00", MaxColor: "#ff0000", Brightness: 75}
	lighting := vuLighting(cfg, 0.375)
	if lighting.GlobalBrightness != 75 {
		t.Fatalf("brillo = %d", lighting.GlobalBrightness)
	}
	if lighting.Knobs[0].Color != "#00ff00" || lighting.Knobs[1].Color != "#2b5500" || lighting.Knobs[2].Color != "#000000" {
		t.Fatalf("segmentos inesperados: %#v", lighting.Knobs)
	}
}

func TestVULightingGradientAtFullScale(t *testing.T) {
	cfg := config.VU{MinColor: "#00ff00", MaxColor: "#ff0000", Brightness: 100}
	lighting := vuLighting(cfg, 1)
	if lighting.Knobs[0].Color != "#00ff00" || lighting.Knobs[3].Color != "#ff0000" {
		t.Fatalf("gradiente inesperado: %#v", lighting.Knobs)
	}
}

func TestSpectrumLightingUsesIndependentBands(t *testing.T) {
	cfg := config.VU{MinColor: "#00ff00", MaxColor: "#ff0000", Brightness: 100}
	lighting := spectrumLighting(cfg, [4]float64{1, 0, 0.5, 0})
	if lighting.Knobs[0].Color != "#00ff00" || lighting.Knobs[1].Color != "#000000" || lighting.Knobs[2].Color != "#552b00" || lighting.Knobs[3].Color != "#000000" {
		t.Fatalf("bandas inesperadas: %#v", lighting.Knobs)
	}
}
