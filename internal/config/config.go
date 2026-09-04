package config

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const Version = 6

const defaultProfileName = "Main"

type Config struct {
	Version       int                `json:"version"`
	API           API                `json:"api"`
	OBS           OBS                `json:"obs"`
	Lighting      Lighting           `json:"lighting"`
	Knobs         [4]Knob            `json:"knobs"`
	ActiveProfile string             `json:"activeProfile"`
	Profiles      map[string]Profile `json:"profiles"`
}

type API struct {
	Token string `json:"token"`
}

type Profile struct {
	Lighting Lighting `json:"lighting"`
	Knobs    [4]Knob  `json:"knobs"`
}

type OBS struct {
	URL      string `json:"url"`
	Password string `json:"password"`
}

type Lighting struct {
	GlobalBrightness int    `json:"globalBrightness"`
	Mode             string `json:"mode"`
	VU               VU     `json:"vu"`
}

type VU struct {
	SourceKind string  `json:"sourceKind"`
	Target     string  `json:"target"`
	MinColor   string  `json:"minColor"`
	MaxColor   string  `json:"maxColor"`
	Brightness int     `json:"brightness"`
	MinDB      float64 `json:"minDb"`
	MaxDB      float64 `json:"maxDb"`
	FPS        int     `json:"fps"`
}

type Knob struct {
	Label string       `json:"label"`
	Light KnobLighting `json:"light"`
	Turn  TurnAction   `json:"turn"`
	Press PressAction  `json:"press"`
}

type KnobLighting struct {
	Color      string `json:"color"`
	TrackValue bool   `json:"trackValue"`
}

type TurnAction struct {
	Kind       string  `json:"kind"`
	Target     string  `json:"target"`
	MinPercent float64 `json:"minPercent"`
	MaxPercent float64 `json:"maxPercent"`
	Source     string  `json:"source,omitempty"`
	Filter     string  `json:"filter,omitempty"`
	Setting    string  `json:"setting,omitempty"`
	MinValue   float64 `json:"minValue,omitempty"`
	MaxValue   float64 `json:"maxValue,omitempty"`
	Command    string  `json:"command,omitempty"`
	RateMS     int     `json:"rateMs,omitempty"`
}

type PressAction struct {
	Kind    string `json:"kind"`
	Target  string `json:"target"`
	Command string `json:"command,omitempty"`
}

// Clone returns an independent copy suitable for editing outside the owner of
// the configuration. Most fields are values already; Profiles is the only map
// and must be copied to avoid concurrent mutation through a shared reference.
func (c Config) Clone() Config {
	clone := c
	if c.Profiles != nil {
		clone.Profiles = make(map[string]Profile, len(c.Profiles))
		for name, profile := range c.Profiles {
			clone.Profiles[name] = profile
		}
	}
	return clone
}

func Default() Config {
	colors := [4]string{"#64e0b1", "#6da8ff", "#b88cff", "#ff9d66"}
	var knobs [4]Knob
	for i := range knobs {
		knobs[i] = Knob{
			Label: fmt.Sprintf("Knob %d", i+1),
			Light: KnobLighting{Color: colors[i]},
			Turn:  TurnAction{Kind: "none", MinPercent: 0, MaxPercent: 100},
			Press: PressAction{Kind: "none"},
		}
	}
	cfg := Config{
		Version: Version,
		API:     API{Token: newToken()},
		OBS:     OBS{URL: "ws://127.0.0.1:4455"},
		Lighting: Lighting{
			GlobalBrightness: 80,
			Mode:             "dials",
			VU:               VU{SourceKind: "output", MinColor: "#35e58a", MaxColor: "#ff4d5f", Brightness: 80, MinDB: -48, MaxDB: 0, FPS: 20},
		},
		Knobs: knobs,
	}
	cfg.ActiveProfile = defaultProfileName
	cfg.Profiles = map[string]Profile{defaultProfileName: {Lighting: cfg.Lighting, Knobs: cfg.Knobs}}
	return cfg
}

func Path() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "panelpc", "config.json"), nil
}

func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Config{}, err
	}
	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("read configuration: %w", err)
	}
	cfg.Normalize()
	return cfg, nil
}

func Save(path string, cfg Config) error {
	cfg.Normalize()
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (c *Config) Normalize() {
	oldVersion := c.Version
	c.Version = Version
	if c.API.Token == "" {
		c.API.Token = newToken()
	}
	if c.OBS.URL == "" {
		c.OBS.URL = "ws://127.0.0.1:4455"
	}
	c.normalizeActive(oldVersion)
	if oldVersion < 4 || len(c.Profiles) == 0 {
		c.ActiveProfile = defaultProfileName
		c.Profiles = map[string]Profile{defaultProfileName: {Lighting: c.Lighting, Knobs: c.Knobs}}
		return
	}
	if oldVersion < 5 {
		if legacy, exists := c.Profiles["Principal"]; exists {
			if _, conflict := c.Profiles[defaultProfileName]; !conflict {
				c.Profiles[defaultProfileName] = legacy
				delete(c.Profiles, "Principal")
				if c.ActiveProfile == "Principal" {
					c.ActiveProfile = defaultProfileName
				}
			}
		}
	}
	if strings.TrimSpace(c.ActiveProfile) == "" {
		c.ActiveProfile = firstProfileName(c.Profiles)
	}
	if _, exists := c.Profiles[c.ActiveProfile]; !exists {
		c.ActiveProfile = firstProfileName(c.Profiles)
	}
	for name, profile := range c.Profiles {
		tmp := Config{Version: Version, Lighting: profile.Lighting, Knobs: profile.Knobs}
		tmp.normalizeActive(Version)
		c.Profiles[name] = Profile{Lighting: tmp.Lighting, Knobs: tmp.Knobs}
	}
	// The root is the active copy used by the engine and interface. On PUT it is
	// also the most recent edit of the selected profile.
	c.Profiles[c.ActiveProfile] = Profile{Lighting: c.Lighting, Knobs: c.Knobs}
}

func newToken() string {
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		panic(fmt.Sprintf("generate API token: %v", err))
	}
	return base64.RawURLEncoding.EncodeToString(secret)
}

func (c *Config) normalizeActive(oldVersion int) {
	if oldVersion < 2 && c.Lighting.GlobalBrightness == 0 {
		c.Lighting.GlobalBrightness = 80
	}
	if c.Lighting.GlobalBrightness < 0 {
		c.Lighting.GlobalBrightness = 0
	}
	if c.Lighting.GlobalBrightness > 100 {
		c.Lighting.GlobalBrightness = 100
	}
	if c.Lighting.Mode != "vu" && c.Lighting.Mode != "spectrum" {
		c.Lighting.Mode = "dials"
	}
	vu := &c.Lighting.VU
	if vu.SourceKind == "" {
		vu.SourceKind = "output"
	}
	if !validColor(vu.MinColor) {
		vu.MinColor = "#35e58a"
	}
	if !validColor(vu.MaxColor) {
		vu.MaxColor = "#ff4d5f"
	}
	if vu.Brightness == 0 && oldVersion < 3 {
		vu.Brightness = 80
	}
	if vu.Brightness < 0 {
		vu.Brightness = 0
	}
	if vu.Brightness > 100 {
		vu.Brightness = 100
	}
	if vu.MinDB == 0 && vu.MaxDB == 0 {
		vu.MinDB, vu.MaxDB = -48, 0
	}
	if vu.MinDB < -100 {
		vu.MinDB = -100
	}
	if vu.MaxDB > 12 {
		vu.MaxDB = 12
	}
	if vu.MaxDB <= vu.MinDB {
		vu.MinDB, vu.MaxDB = -48, 0
	}
	if vu.FPS == 0 {
		vu.FPS = 20
	}
	if vu.FPS < 5 {
		vu.FPS = 5
	}
	if vu.FPS > 30 {
		vu.FPS = 30
	}
	colors := [4]string{"#64e0b1", "#6da8ff", "#b88cff", "#ff9d66"}
	for i := range c.Knobs {
		if c.Knobs[i].Label == "" {
			c.Knobs[i].Label = fmt.Sprintf("Knob %d", i+1)
		}
		if !validColor(c.Knobs[i].Light.Color) {
			c.Knobs[i].Light.Color = colors[i]
		}
		t := &c.Knobs[i].Turn
		if t.Kind == "" {
			t.Kind = "none"
		}
		if t.MaxPercent == 0 && t.MinPercent == 0 {
			t.MaxPercent = 100
		}
		if t.MinPercent < 0 {
			t.MinPercent = 0
		}
		if t.MinPercent > 150 {
			t.MinPercent = 150
		}
		if t.MaxPercent < 0 {
			t.MaxPercent = 0
		}
		if t.MaxPercent > 150 {
			t.MaxPercent = 150
		}
		if t.MaxPercent < t.MinPercent {
			t.MinPercent, t.MaxPercent = t.MaxPercent, t.MinPercent
		}
		if t.Kind == "obs_filter" {
			if t.Setting == "" {
				t.Setting = "opacity"
			}
			if t.MinValue == 0 && t.MaxValue == 0 {
				t.MaxValue = 1
			}
			if t.MaxValue < t.MinValue {
				t.MinValue, t.MaxValue = t.MaxValue, t.MinValue
			}
		}
		if t.Kind == "shell" {
			if t.RateMS == 0 {
				t.RateMS = 250
			}
			if t.RateMS < 50 {
				t.RateMS = 50
			}
			if t.RateMS > 60000 {
				t.RateMS = 60000
			}
		}
		if c.Knobs[i].Press.Kind == "" {
			c.Knobs[i].Press.Kind = "none"
		}
	}
}

func (c *Config) ActivateProfile(name string) error {
	name = strings.TrimSpace(name)
	profile, exists := c.Profiles[name]
	if !exists {
		return fmt.Errorf("profile %q does not exist", name)
	}
	c.ActiveProfile = name
	c.Lighting = profile.Lighting
	c.Knobs = profile.Knobs
	c.Normalize()
	return nil
}

func (c Config) ProfileNames() []string {
	names := make([]string, 0, len(c.Profiles))
	for name := range c.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func firstProfileName(profiles map[string]Profile) string {
	names := make([]string, 0, len(profiles))
	for name := range profiles {
		if strings.TrimSpace(name) != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return defaultProfileName
	}
	sort.Strings(names)
	return names[0]
}

func validColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, char := range color[1:] {
		if !((char >= '0' && char <= '9') || (char >= 'a' && char <= 'f') || (char >= 'A' && char <= 'F')) {
			return false
		}
	}
	return true
}
