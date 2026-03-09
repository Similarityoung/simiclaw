package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultLogLevel(t *testing.T) {
	cfg := Default()
	if cfg.LogLevel != "info" {
		t.Fatalf("unexpected default log level: %s", cfg.LogLevel)
	}
}

func TestDefaultTelegramConfig(t *testing.T) {
	cfg := Default()
	if cfg.Channels.Telegram.Enabled {
		t.Fatal("telegram should be disabled by default")
	}
	if cfg.Channels.Telegram.LongPollTimeout.Duration != 30*time.Second {
		t.Fatalf("unexpected telegram long poll timeout: %v", cfg.Channels.Telegram.LongPollTimeout.Duration)
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

func TestLoadTelegramEnabledWithoutTokenFails(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte(`{
		"workspace": ".",
		"channels": {
			"telegram": {
				"enabled": true,
				"allowed_user_ids": [1001]
			}
		}
	}`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(configPath); err == nil {
		t.Fatal("expected telegram token validation error")
	}
}

func TestLoadEnvOpenAIAliases(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("SIMICLAW_LLM_DEFAULT_MODEL", "")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("LLM_BASE_URL", "https://api.deepseek.com")
	t.Setenv("LLM_MODEL", "openai/deepseek-chat")
	t.Setenv("TELEGRAM_TOKEN", "")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config from env: %v", err)
	}
	if cfg.LLM.DefaultModel != "openai/deepseek-chat" {
		t.Fatalf("unexpected default model: %s", cfg.LLM.DefaultModel)
	}
	provider, ok := cfg.LLM.Providers["openai"]
	if !ok {
		t.Fatal("expected openai provider from env aliases")
	}
	if provider.Type != "openai_compatible" {
		t.Fatalf("unexpected provider type: %s", provider.Type)
	}
	if provider.APIKey != "test-key" {
		t.Fatalf("unexpected api key: %s", provider.APIKey)
	}
	if provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("unexpected base url: %s", provider.BaseURL)
	}
}

func TestLoadTelegramTokenFromEnv(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("SIMICLAW_LLM_DEFAULT_MODEL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("TELEGRAM_TOKEN", "env-telegram-token")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte(`{
		"workspace": ".",
		"channels": {
			"telegram": {
				"enabled": true,
				"allowed_user_ids": [1001],
				"token": "json-token"
			}
		}
	}`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.Channels.Telegram.Token != "env-telegram-token" {
		t.Fatalf("unexpected telegram token: %s", cfg.Channels.Telegram.Token)
	}
}
