package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

// Duration wraps time.Duration with human-readable JSON (e.g. "200ms", "5s").
type Duration struct {
	time.Duration
}

func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

func (d *Duration) UnmarshalJSON(b []byte) error {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	switch val := v.(type) {
	case string:
		parsed, err := time.ParseDuration(val)
		if err != nil {
			return err
		}
		d.Duration = parsed
	case float64:
		d.Duration = time.Duration(int64(val))
	default:
		return errors.New("invalid duration value")
	}
	return nil
}

type Config struct {
	Workspace             string        `json:"workspace"`
	ListenAddr            string        `json:"listen_addr"`
	APIKey                string        `json:"api_key"`
	TenantID              string        `json:"tenant_id"`
	EventQueueCapacity    int           `json:"event_queue_capacity"`
	IngestEnqueueTimeout  Duration      `json:"ingest_enqueue_timeout"`
	RateLimitTenantRPS    float64       `json:"rate_limit_tenant_rps"`
	RateLimitTenantBurst  float64       `json:"rate_limit_tenant_burst"`
	RateLimitSessionRPS   float64       `json:"rate_limit_session_rps"`
	RateLimitSessionBurst float64       `json:"rate_limit_session_burst"`
	MaxToolRounds         int           `json:"max_tool_rounds"`
}

func Default() Config {
	return Config{
		Workspace:             ".",
		ListenAddr:            ":8080",
		TenantID:              "local",
		EventQueueCapacity:    1024,
		IngestEnqueueTimeout:  Duration{200 * time.Millisecond},
		RateLimitTenantRPS:    30,
		RateLimitTenantBurst:  60,
		RateLimitSessionRPS:   5,
		RateLimitSessionBurst: 10,
		MaxToolRounds:         4,
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		return cfg, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	if cfg.Workspace == "" {
		return cfg, errors.New("workspace is required")
	}
	cfg.Workspace = filepath.Clean(cfg.Workspace)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = ":8080"
	}
	if cfg.TenantID == "" {
		cfg.TenantID = "local"
	}
	if cfg.EventQueueCapacity <= 0 {
		cfg.EventQueueCapacity = 1024
	}
	if cfg.IngestEnqueueTimeout.Duration <= 0 {
		cfg.IngestEnqueueTimeout = Duration{200 * time.Millisecond}
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = 4
	}
	if cfg.RateLimitTenantRPS <= 0 {
		cfg.RateLimitTenantRPS = 30
	}
	if cfg.RateLimitTenantBurst <= 0 {
		cfg.RateLimitTenantBurst = 60
	}
	if cfg.RateLimitSessionRPS <= 0 {
		cfg.RateLimitSessionRPS = 5
	}
	if cfg.RateLimitSessionBurst <= 0 {
		cfg.RateLimitSessionBurst = 10
	}
	return cfg, nil
}
