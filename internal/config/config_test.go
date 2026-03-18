package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := Default()

	// Top-level fields
	if cfg.IdleTimeout != "30m" {
		t.Errorf("IdleTimeout: got %q, want %q", cfg.IdleTimeout, "30m")
	}
	if cfg.LogBufferSize != 2000 {
		t.Errorf("LogBufferSize: got %d, want %d", cfg.LogBufferSize, 2000)
	}

	// WDA defaults
	if !cfg.WDA.AutoStart {
		t.Error("WDA.AutoStart: got false, want true")
	}
	if cfg.WDA.BundleID != "com.facebook.WebDriverAgentRunner.xctrunner" {
		t.Errorf("WDA.BundleID: got %q, want %q", cfg.WDA.BundleID, "com.facebook.WebDriverAgentRunner.xctrunner")
	}
	if cfg.WDA.HealthInterval != "30s" {
		t.Errorf("WDA.HealthInterval: got %q, want %q", cfg.WDA.HealthInterval, "30s")
	}
	if cfg.WDA.MaxRestart != 3 {
		t.Errorf("WDA.MaxRestart: got %d, want %d", cfg.WDA.MaxRestart, 3)
	}

	// Screenshot defaults
	if cfg.Screenshot.RetentionHours != 24 {
		t.Errorf("Screenshot.RetentionHours: got %d, want %d", cfg.Screenshot.RetentionHours, 24)
	}
	if cfg.Screenshot.MaxCount != 200 {
		t.Errorf("Screenshot.MaxCount: got %d, want %d", cfg.Screenshot.MaxCount, 200)
	}

	// Annotate defaults
	if cfg.Annotate.BoxColor != "#FF0000" {
		t.Errorf("Annotate.BoxColor: got %q, want %q", cfg.Annotate.BoxColor, "#FF0000")
	}
	if cfg.Annotate.LabelSize != 14 {
		t.Errorf("Annotate.LabelSize: got %d, want %d", cfg.Annotate.LabelSize, 14)
	}
	expectedTypes := []string{"button", "textfield", "switch", "link", "cell"}
	if len(cfg.Annotate.InteractiveTypes) != len(expectedTypes) {
		t.Errorf("Annotate.InteractiveTypes length: got %d, want %d", len(cfg.Annotate.InteractiveTypes), len(expectedTypes))
	} else {
		for i, v := range expectedTypes {
			if cfg.Annotate.InteractiveTypes[i] != v {
				t.Errorf("Annotate.InteractiveTypes[%d]: got %q, want %q", i, cfg.Annotate.InteractiveTypes[i], v)
			}
		}
	}
}

func TestLoadFromFile(t *testing.T) {
	data := `{
		"idle_timeout": "1h",
		"log_buffer_size": 500
	}`
	f, err := os.CreateTemp("", "ios-pilot-config-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadFrom(f.Name())
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}

	// Overridden fields
	if cfg.IdleTimeout != "1h" {
		t.Errorf("IdleTimeout: got %q, want %q", cfg.IdleTimeout, "1h")
	}
	if cfg.LogBufferSize != 500 {
		t.Errorf("LogBufferSize: got %d, want %d", cfg.LogBufferSize, 500)
	}

	// Unspecified fields should retain defaults
	if !cfg.WDA.AutoStart {
		t.Error("WDA.AutoStart: got false, want true (default)")
	}
	if cfg.WDA.BundleID != "com.facebook.WebDriverAgentRunner.xctrunner" {
		t.Errorf("WDA.BundleID: got %q, want default", cfg.WDA.BundleID)
	}
}

func TestLoadMissingFile(t *testing.T) {
	cfg, err := LoadFrom(filepath.Join(os.TempDir(), "nonexistent-ios-pilot-config.json"))
	if err != nil {
		t.Fatalf("LoadFrom missing file returned error: %v", err)
	}
	// Should return defaults
	def := Default()
	if cfg.IdleTimeout != def.IdleTimeout {
		t.Errorf("IdleTimeout: got %q, want %q", cfg.IdleTimeout, def.IdleTimeout)
	}
	if cfg.WDA.MaxRestart != def.WDA.MaxRestart {
		t.Errorf("WDA.MaxRestart: got %d, want %d", cfg.WDA.MaxRestart, def.WDA.MaxRestart)
	}
}

func TestPartialNestedOverride(t *testing.T) {
	data := `{
		"wda": {
			"bundle_id": "com.custom.wda.xctrunner"
		}
	}`
	f, err := os.CreateTemp("", "ios-pilot-config-partial-*.json")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(f.Name())
	if _, err := f.WriteString(data); err != nil {
		t.Fatal(err)
	}
	f.Close()

	cfg, err := LoadFrom(f.Name())
	if err != nil {
		t.Fatalf("LoadFrom returned error: %v", err)
	}

	// Overridden field
	if cfg.WDA.BundleID != "com.custom.wda.xctrunner" {
		t.Errorf("WDA.BundleID: got %q, want %q", cfg.WDA.BundleID, "com.custom.wda.xctrunner")
	}

	// Other WDA fields must retain defaults, NOT be zero-valued
	if !cfg.WDA.AutoStart {
		t.Error("WDA.AutoStart: got false, want true (default must be preserved)")
	}
	if cfg.WDA.HealthInterval != "30s" {
		t.Errorf("WDA.HealthInterval: got %q, want %q (default must be preserved)", cfg.WDA.HealthInterval, "30s")
	}
	if cfg.WDA.MaxRestart != 3 {
		t.Errorf("WDA.MaxRestart: got %d, want 3 (default must be preserved)", cfg.WDA.MaxRestart)
	}

	// Top-level fields must also retain defaults
	if cfg.IdleTimeout != "30m" {
		t.Errorf("IdleTimeout: got %q, want %q (default)", cfg.IdleTimeout, "30m")
	}

	// Annotate defaults retained
	if cfg.Annotate.BoxColor != "#FF0000" {
		t.Errorf("Annotate.BoxColor: got %q, want default", cfg.Annotate.BoxColor)
	}
}

func TestConfigRoundTrip(t *testing.T) {
	cfg := Default()
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var cfg2 Config
	if err := json.Unmarshal(data, &cfg2); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if cfg.IdleTimeout != cfg2.IdleTimeout {
		t.Errorf("round-trip IdleTimeout mismatch: %q vs %q", cfg.IdleTimeout, cfg2.IdleTimeout)
	}
	if cfg.WDA.BundleID != cfg2.WDA.BundleID {
		t.Errorf("round-trip WDA.BundleID mismatch")
	}
}
