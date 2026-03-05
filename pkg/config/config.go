package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
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

	// LLM 配置：接入 OpenAI 兼容 API（GPT-4、DeepSeek、Ollama 等）
	LLMBaseURL string   `json:"llm_base_url,omitempty"`
	LLMAPIKey  string   `json:"llm_api_key,omitempty"`
	LLMModel   string   `json:"llm_model,omitempty"`
	LLMTimeout Duration `json:"llm_timeout,omitempty"`
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
	defaultLLMBaseURL            = "https://api.openai.com/v1"
	defaultLLMModel              = "gpt-4o"
	defaultLLMTimeout            = 60 * time.Second
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
		LLMBaseURL:            defaultLLMBaseURL,
		LLMModel:              defaultLLMModel,
		LLMTimeout:            Duration{defaultLLMTimeout},
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
	if err := validateLogLevel(cfg.LogLevel); err != nil {
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
	if cfg.LLMBaseURL == "" {
		cfg.LLMBaseURL = defaultLLMBaseURL
	}
	if cfg.LLMModel == "" {
		cfg.LLMModel = defaultLLMModel
	}
	if cfg.LLMTimeout.Duration <= 0 {
		cfg.LLMTimeout = Duration{defaultLLMTimeout}
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
	applyEnvOverrides(&cfg)
	return cfg, nil
}

func validateLogLevel(raw string) error {
	level := strings.ToLower(strings.TrimSpace(raw))
	switch level {
	case "debug", "info", "warn", "error":
		return nil
	default:
		return fmt.Errorf("invalid log_level %q: must be one of debug|info|warn|error", raw)
	}
}

// LoadDotEnv 解析 .env 文件并将 KEY=VALUE 注入环境变量。
// 若文件不存在则静默跳过（.env 为可选）；已有的环境变量不会被覆盖。
func LoadDotEnv(path string) error {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("config: open .env %q: %w", path, err)
	}
	defer func() { _ = f.Close() }()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		// 去掉可选的引号包裹（单引号或双引号）
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if k == "" {
			continue
		}
		// 真实环境变量优先级高于 .env 文件
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return scanner.Err()
}

// applyEnvOverrides 将环境变量中的 LLM 配置覆盖到 cfg。
// 优先级：环境变量 > JSON 配置文件 > 默认值。
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("LLM_API_KEY"); v != "" {
		cfg.LLMAPIKey = v
	}
	if v := os.Getenv("LLM_BASE_URL"); v != "" {
		cfg.LLMBaseURL = v
	}
	if v := os.Getenv("LLM_MODEL"); v != "" {
		cfg.LLMModel = v
	}
}
