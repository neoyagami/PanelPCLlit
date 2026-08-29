package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Default()
	want.Knobs[0].Label = "Música"
	want.Knobs[0].Turn = TurnAction{Kind: "app", Target: "Spotify", MinPercent: 5, MaxPercent: 90}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permisos = %o, se esperaba 600", info.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Knobs[0] != want.Knobs[0] {
		t.Fatalf("round trip distinto: %#v != %#v", got.Knobs[0], want.Knobs[0])
	}
}

func TestNormalizeLimits(t *testing.T) {
	cfg := Default()
	cfg.Knobs[0].Turn.MinPercent = 120
	cfg.Knobs[0].Turn.MaxPercent = -10
	cfg.Normalize()
	if cfg.Knobs[0].Turn.MinPercent != 0 || cfg.Knobs[0].Turn.MaxPercent != 120 {
		t.Fatalf("rango inesperado: %#v", cfg.Knobs[0].Turn)
	}
}

func TestVersionOneMigratesLightingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := []byte(`{"version":1,"obs":{"url":"ws://127.0.0.1:4455"},"knobs":[{"label":"Uno","turn":{"kind":"none"},"press":{"kind":"none"}},{},{},{}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != Version || cfg.Lighting.GlobalBrightness != 80 || cfg.Lighting.VU.Brightness != 80 || cfg.Knobs[0].Light.Color == "" {
		t.Fatalf("migración incompleta: %#v", cfg)
	}
}

func TestVersionTwoMigratesVUDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := []byte(`{"version":2,"lighting":{"globalBrightness":55},"knobs":[{},{},{},{}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Lighting.Mode != "dials" || cfg.Lighting.VU.Brightness != 80 || cfg.Lighting.VU.MinDB != -48 || cfg.Lighting.VU.FPS != 20 {
		t.Fatalf("migración VU incompleta: %#v", cfg.Lighting)
	}
}

func TestNormalizeShellRate(t *testing.T) {
	cfg := Default()
	cfg.Knobs[0].Turn = TurnAction{Kind: "shell", RateMS: 1}
	cfg.Knobs[1].Turn = TurnAction{Kind: "shell", RateMS: 70000}
	cfg.Normalize()
	if cfg.Knobs[0].Turn.RateMS != 50 || cfg.Knobs[1].Turn.RateMS != 60000 {
		t.Fatalf("rates inesperados: %d, %d", cfg.Knobs[0].Turn.RateMS, cfg.Knobs[1].Turn.RateMS)
	}
}

func TestActivateProfile(t *testing.T) {
	cfg := Default()
	night := cfg.Profiles["Principal"]
	night.Knobs[0].Label = "Noche"
	night.Lighting.Mode = "vu"
	cfg.Profiles["Noche"] = night
	if err := cfg.ActivateProfile("Noche"); err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "Noche" || cfg.Knobs[0].Label != "Noche" || cfg.Lighting.Mode != "vu" {
		t.Fatalf("perfil no activado: %#v", cfg)
	}
	if err := cfg.ActivateProfile("No existe"); err == nil {
		t.Fatal("se esperaba error para un perfil inexistente")
	}
}
