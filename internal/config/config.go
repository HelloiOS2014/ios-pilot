// Package config handles ios-pilot daemon configuration loading and defaults.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Config is the top-level configuration for ios-pilot.
type Config struct {
	IdleTimeout   string           `json:"idle_timeout"`
	LogBufferSize int              `json:"log_buffer_size"`
	WDA           WDAConfig        `json:"wda"`
	Screenshot    ScreenshotConfig `json:"screenshot"`
	Annotate      AnnotateConfig   `json:"annotate"`
}

// WDAConfig holds WebDriverAgent configuration.
type WDAConfig struct {
	AutoStart      bool   `json:"auto_start"`
	BundleID       string `json:"bundle_id"`
	HealthInterval string `json:"health_interval"`
	MaxRestart     int    `json:"max_restart"`
}

// ScreenshotConfig holds screenshot management configuration.
type ScreenshotConfig struct {
	Dir            string `json:"dir"`
	RetentionHours int    `json:"retention_hours"`
	MaxCount       int    `json:"max_count"`
}

// AnnotateConfig holds UI annotation configuration.
type AnnotateConfig struct {
	BoxColor         string   `json:"box_color"`
	LabelSize        int      `json:"label_size"`
	InteractiveTypes []string `json:"interactive_types"`
}

// Default returns a Config populated with all default values.
func Default() Config {
	home, _ := os.UserHomeDir()
	screenshotDir := filepath.Join(home, ".config", "ios-pilot", "screenshots")

	return Config{
		IdleTimeout:   "30m",
		LogBufferSize: 2000,
		WDA: WDAConfig{
			AutoStart:      true,
			BundleID:       "com.facebook.WebDriverAgentRunner.xctrunner",
			HealthInterval: "30s",
			MaxRestart:     3,
		},
		Screenshot: ScreenshotConfig{
			Dir:            screenshotDir,
			RetentionHours: 24,
			MaxCount:       200,
		},
		Annotate: AnnotateConfig{
			BoxColor:  "#FF0000",
			LabelSize: 14,
			InteractiveTypes: []string{
				"button", "textfield", "switch", "link", "cell", "icon", "text",
			},
		},
	}
}

// LoadFrom loads configuration from the given file path, merging over defaults.
// If the file does not exist, defaults are returned without error.
//
// Partial nested struct overrides are handled correctly: specifying only
// {"wda":{"bundle_id":"custom"}} will preserve all other WDA defaults.
func LoadFrom(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}

	// Unmarshal into a raw map to detect which top-level keys are present.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return cfg, err
	}

	// Merge top-level scalar / string fields.
	if v, ok := raw["idle_timeout"]; ok {
		json.Unmarshal(v, &cfg.IdleTimeout)
	}
	if v, ok := raw["log_buffer_size"]; ok {
		json.Unmarshal(v, &cfg.LogBufferSize)
	}

	// Merge nested structs: unmarshal only into the existing (defaulted) struct
	// so that omitted fields retain their defaults.
	if v, ok := raw["wda"]; ok {
		json.Unmarshal(v, &cfg.WDA)
	}
	if v, ok := raw["screenshot"]; ok {
		json.Unmarshal(v, &cfg.Screenshot)
	}
	if v, ok := raw["annotate"]; ok {
		json.Unmarshal(v, &cfg.Annotate)
	}

	return cfg, nil
}

// Load loads configuration from the default path:
// ~/.config/ios-pilot/config.json, falling back to defaults.
func Load() (Config, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Default(), err
	}
	return LoadFrom(filepath.Join(home, ".config", "ios-pilot", "config.json"))
}
