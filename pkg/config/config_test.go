package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultLogLevel(t *testing.T) {
	cfg := Default()
	if cfg.LogLevel != "info" {
		t.Fatalf("unexpected default log level: %s", cfg.LogLevel)
	}
}

func TestLoadLogLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte("{\"workspace\":\".\",\"log_level\":\"warn\"}")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.LogLevel != "warn" {
		t.Fatalf("unexpected log level: %s", cfg.LogLevel)
	}
}

func TestLoadInvalidLogLevel(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte("{\"workspace\":\".\",\"log_level\":\"verbose\"}")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("expected invalid log_level error")
	}
}

func TestLoadIgnoresRemovedEnableADKGatewayField(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte("{\"workspace\":\".\",\"enable_adk_gateway\":true}")
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Workspace != "." {
		t.Fatalf("unexpected workspace: %s", cfg.Workspace)
	}
}
