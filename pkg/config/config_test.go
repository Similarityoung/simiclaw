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

func TestDefaultWebSearchConfig(t *testing.T) {
	cfg := Default()
	if cfg.WebSearch.Timeout.Duration != 10*time.Second {
		t.Fatalf("unexpected web search timeout: %v", cfg.WebSearch.Timeout.Duration)
	}
	if cfg.WebSearch.MaxResults != 5 {
		t.Fatalf("unexpected web search max results: %d", cfg.WebSearch.MaxResults)
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
	setTelegramEnv(t, "", "", "", "")
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
	setLLMEnv(t)
	setTelegramEnv(t, "", "env-telegram-token", "", "")

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

func TestLoadTelegramFieldsFromEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "true", "env-telegram-token", "1001, 2002", "45s")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config from env: %v", err)
	}
	if !cfg.Channels.Telegram.Enabled {
		t.Fatal("expected telegram enabled from env")
	}
	if cfg.Channels.Telegram.Token != "env-telegram-token" {
		t.Fatalf("unexpected telegram token: %s", cfg.Channels.Telegram.Token)
	}
	if len(cfg.Channels.Telegram.AllowedUserIDs) != 2 || cfg.Channels.Telegram.AllowedUserIDs[0] != 1001 || cfg.Channels.Telegram.AllowedUserIDs[1] != 2002 {
		t.Fatalf("unexpected allowed user ids: %+v", cfg.Channels.Telegram.AllowedUserIDs)
	}
	if cfg.Channels.Telegram.LongPollTimeout.Duration != 45*time.Second {
		t.Fatalf("unexpected telegram long poll timeout: %v", cfg.Channels.Telegram.LongPollTimeout.Duration)
	}
}

func TestLoadInvalidTelegramEnabledEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "maybe", "env-telegram-token", "", "")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid TELEGRAM_ENABLED error")
	}
}

func TestLoadInvalidTelegramAllowedUserIDsEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "true", "env-telegram-token", "1001, nope", "")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid TELEGRAM_ALLOWED_USER_IDS error")
	}
}

func TestLoadInvalidTelegramLongPollTimeoutEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "true", "env-telegram-token", "1001", "later")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid TELEGRAM_LONG_POLL_TIMEOUT error")
	}
}

func TestLoadWebSearchFieldsFromEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "", "", "", "")
	t.Setenv("WEB_SEARCH_TIMEOUT", "12s")
	t.Setenv("WEB_SEARCH_MAX_RESULTS", "7")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config from env: %v", err)
	}
	if cfg.WebSearch.Timeout.Duration != 12*time.Second {
		t.Fatalf("unexpected web search timeout: %v", cfg.WebSearch.Timeout.Duration)
	}
	if cfg.WebSearch.MaxResults != 7 {
		t.Fatalf("unexpected web search max results: %d", cfg.WebSearch.MaxResults)
	}
}

func TestLoadWebSearchMaxResultsClamp(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "", "", "", "")
	t.Setenv("WEB_SEARCH_TIMEOUT", "")
	t.Setenv("WEB_SEARCH_MAX_RESULTS", "99")

	cfg, err := Load("")
	if err != nil {
		t.Fatalf("load config from env: %v", err)
	}
	if cfg.WebSearch.MaxResults != 8 {
		t.Fatalf("unexpected clamped web search max results: %d", cfg.WebSearch.MaxResults)
	}
}

func TestLoadWebSearchMaxResultsClampFromJSON(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "", "", "", "")

	dir := t.TempDir()
	configPath := filepath.Join(dir, "cfg.json")
	data := []byte(`{"workspace":".","web_search":{"max_results":-1}}`)
	if err := os.WriteFile(configPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.WebSearch.MaxResults != 5 {
		t.Fatalf("unexpected defaulted web search max results: %d", cfg.WebSearch.MaxResults)
	}
}

func TestLoadInvalidWebSearchTimeoutEnv(t *testing.T) {
	setLLMEnv(t)
	setTelegramEnv(t, "", "", "", "")
	t.Setenv("WEB_SEARCH_TIMEOUT", "later")

	if _, err := Load(""); err == nil {
		t.Fatal("expected invalid WEB_SEARCH_TIMEOUT error")
	}
}

func setLLMEnv(t *testing.T) {
	t.Helper()
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("SIMICLAW_LLM_DEFAULT_MODEL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL", "")
}

func setTelegramEnv(t *testing.T, enabled, token, allowedUserIDs, longPollTimeout string) {
	t.Helper()
	t.Setenv("TELEGRAM_ENABLED", enabled)
	t.Setenv("TELEGRAM_TOKEN", token)
	t.Setenv("TELEGRAM_ALLOWED_USER_IDS", allowedUserIDs)
	t.Setenv("TELEGRAM_LONG_POLL_TIMEOUT", longPollTimeout)
}
