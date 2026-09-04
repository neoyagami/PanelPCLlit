package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "config.json")
	want := Default()
	want.Knobs[0].Label = "Music"
	want.Knobs[0].Turn = TurnAction{Kind: "app", Target: "Spotify", MinPercent: 5, MaxPercent: 90}
	if err := Save(path, want); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %o, expected 600", info.Mode().Perm())
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Knobs[0] != want.Knobs[0] {
		t.Fatalf("round trip differs: %#v != %#v", got.Knobs[0], want.Knobs[0])
	}
	if got.API.Token == "" || got.API.Token != want.API.Token {
		t.Fatalf("API token was not persisted")
	}
}

func TestNormalizeLimits(t *testing.T) {
	cfg := Default()
	cfg.Knobs[0].Turn.MinPercent = 120
	cfg.Knobs[0].Turn.MaxPercent = -10
	cfg.Normalize()
	if cfg.Knobs[0].Turn.MinPercent != 0 || cfg.Knobs[0].Turn.MaxPercent != 120 {
		t.Fatalf("unexpected range: %#v", cfg.Knobs[0].Turn)
	}
}

func TestVersionOneMigratesLightingDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := []byte(`{"version":1,"obs":{"url":"ws://127.0.0.1:4455"},"knobs":[{"label":"One","turn":{"kind":"none"},"press":{"kind":"none"}},{},{},{}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Version != Version || cfg.Lighting.GlobalBrightness != 80 || cfg.Lighting.VU.Brightness != 80 || cfg.Knobs[0].Light.Color == "" {
		t.Fatalf("incomplete migration: %#v", cfg)
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
		t.Fatalf("incomplete VU migration: %#v", cfg.Lighting)
	}
}

func TestNormalizeShellRate(t *testing.T) {
	cfg := Default()
	cfg.Knobs[0].Turn = TurnAction{Kind: "shell", RateMS: 1}
	cfg.Knobs[1].Turn = TurnAction{Kind: "shell", RateMS: 70000}
	cfg.Normalize()
	if cfg.Knobs[0].Turn.RateMS != 50 || cfg.Knobs[1].Turn.RateMS != 60000 {
		t.Fatalf("unexpected rates: %d, %d", cfg.Knobs[0].Turn.RateMS, cfg.Knobs[1].Turn.RateMS)
	}
}

func TestCloneDoesNotShareProfiles(t *testing.T) {
	cfg := Default()
	clone := cfg.Clone()
	profile := clone.Profiles["Main"]
	profile.Knobs[0].Label = "Edited"
	clone.Profiles["Main"] = profile
	delete(clone.Profiles, "unused")

	if got := cfg.Profiles["Main"].Knobs[0].Label; got == "Edited" {
		t.Fatal("Clone shares its Profiles map with the original")
	}
}

func TestActivateProfile(t *testing.T) {
	cfg := Default()
	night := cfg.Profiles[defaultProfileName]
	night.Knobs[0].Label = "Night"
	night.Lighting.Mode = "vu"
	cfg.Profiles["Night"] = night
	if err := cfg.ActivateProfile("Night"); err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != "Night" || cfg.Knobs[0].Label != "Night" || cfg.Lighting.Mode != "vu" {
		t.Fatalf("profile was not activated: %#v", cfg)
	}
	if err := cfg.ActivateProfile("Does not exist"); err == nil {
		t.Fatal("expected an error for a nonexistent profile")
	}
}

func TestVersionFourRenamesLegacyDefaultProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	legacy := []byte(`{"version":4,"activeProfile":"Principal","profiles":{"Principal":{"lighting":{"globalBrightness":80,"mode":"dials"},"knobs":[{},{},{},{}]}},"lighting":{"globalBrightness":80,"mode":"dials"},"knobs":[{},{},{},{}]}`)
	if err := os.WriteFile(path, legacy, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveProfile != defaultProfileName {
		t.Fatalf("active profile = %q, want %q", cfg.ActiveProfile, defaultProfileName)
	}
	if _, exists := cfg.Profiles["Principal"]; exists {
		t.Fatal("legacy default profile was not renamed")
	}
}
