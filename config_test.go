package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfigFileRefusesOverwriteWithoutForce(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := defaultRuntimeConfig()
	cfg.APIKey = "secret"

	if err := writeConfigFile(path, cfg, false); err != nil {
		t.Fatalf("first writeConfigFile returned error: %v", err)
	}
	if err := writeConfigFile(path, cfg, false); err == nil {
		t.Fatal("expected overwrite without force to fail")
	}
}

func TestWriteConfigFileClearsAPIKey(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := defaultRuntimeConfig()
	cfg.APIKey = "secret"
	cfg.DistanceUnit = "mi"

	if err := writeConfigFile(path, cfg, true); err != nil {
		t.Fatalf("writeConfigFile returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}
	text := string(data)
	if strings.Contains(text, "secret") {
		t.Fatalf("config file unexpectedly contained API key: %s", text)
	}
	if !strings.Contains(text, "distance_unit: mi") {
		t.Fatalf("config file did not include expected distance unit: %s", text)
	}
}

func TestInitDistanceUnitDefault(t *testing.T) {
	t.Parallel()

	if got := initDistanceUnitDefault(defaultRuntimeConfig()); got != "mi" {
		t.Fatalf("initDistanceUnitDefault(defaultRuntimeConfig()) = %q, want mi", got)
	}

	cfg := defaultRuntimeConfig()
	cfg.DistanceUnit = "km"
	if got := initDistanceUnitDefault(cfg); got != "km" {
		t.Fatalf("initDistanceUnitDefault(cfg) = %q, want km", got)
	}
}
