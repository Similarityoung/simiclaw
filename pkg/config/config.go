package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/logging"
)

// Duration wraps time.Duration with human-readable JSON (e.g. "200ms", "5s").
type Duration struct {
	time.Duration
}

// MarshalJSON 将 Duration 序列化为可读的时长字符串。
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(d.Duration.String())
}

// UnmarshalJSON 支持从字符串或数字反序列化 Duration。
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
	Workspace             string   `json:"workspace"`
	ListenAddr            string   `json:"listen_addr"`
	LogLevel              string   `json:"log_level"`
	APIKey                string   `json:"api_key"`
	TenantID              string   `json:"tenant_id"`
	EventQueueCapacity    int      `json:"event_queue_capacity"`
	IngestEnqueueTimeout  Duration `json:"ingest_enqueue_timeout"`
	RateLimitTenantRPS    float64  `json:"rate_limit_tenant_rps"`
	RateLimitTenantBurst  float64  `json:"rate_limit_tenant_burst"`
	RateLimitSessionRPS   float64  `json:"rate_limit_session_rps"`
	RateLimitSessionBurst float64  `json:"rate_limit_session_burst"`
	MaxToolRounds         int      `json:"max_tool_rounds"`
}

const (
	defaultWorkspace             = "."
	defaultListenAddr            = ":8080"
	defaultLogLevel              = "info"
	defaultTenantID              = "local"
	defaultEventQueueCapacity    = 1024
	defaultIngestEnqueueTimeout  = 200 * time.Millisecond
	defaultRateLimitTenantRPS    = 30
	defaultRateLimitTenantBurst  = 60
	defaultRateLimitSessionRPS   = 5
	defaultRateLimitSessionBurst = 10
	defaultMaxToolRounds         = 4
)

// Default 返回服务配置的默认值。
func Default() Config {
	return Config{
		Workspace:             defaultWorkspace,
		ListenAddr:            defaultListenAddr,
		LogLevel:              defaultLogLevel,
		TenantID:              defaultTenantID,
		EventQueueCapacity:    defaultEventQueueCapacity,
		IngestEnqueueTimeout:  Duration{defaultIngestEnqueueTimeout},
		RateLimitTenantRPS:    defaultRateLimitTenantRPS,
		RateLimitTenantBurst:  defaultRateLimitTenantBurst,
		RateLimitSessionRPS:   defaultRateLimitSessionRPS,
		RateLimitSessionBurst: defaultRateLimitSessionBurst,
		MaxToolRounds:         defaultMaxToolRounds,
	}
}

// Load 从 JSON 文件加载配置，并补齐缺省值与基础校验。
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
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
	}
	if _, err := logging.ParseLevel(cfg.LogLevel); err != nil {
		return cfg, err
	}
	if cfg.TenantID == "" {
		cfg.TenantID = defaultTenantID
	}
	if cfg.EventQueueCapacity <= 0 {
		cfg.EventQueueCapacity = defaultEventQueueCapacity
	}
	if cfg.IngestEnqueueTimeout.Duration <= 0 {
		cfg.IngestEnqueueTimeout = Duration{defaultIngestEnqueueTimeout}
	}
	if cfg.MaxToolRounds <= 0 {
		cfg.MaxToolRounds = defaultMaxToolRounds
	}
	if cfg.RateLimitTenantRPS <= 0 {
		cfg.RateLimitTenantRPS = defaultRateLimitTenantRPS
	}
	if cfg.RateLimitTenantBurst <= 0 {
		cfg.RateLimitTenantBurst = defaultRateLimitTenantBurst
	}
	if cfg.RateLimitSessionRPS <= 0 {
		cfg.RateLimitSessionRPS = defaultRateLimitSessionRPS
	}
	if cfg.RateLimitSessionBurst <= 0 {
		cfg.RateLimitSessionBurst = defaultRateLimitSessionBurst
	}
	return cfg, nil
}
