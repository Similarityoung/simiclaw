package config

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

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

type LLMConfig struct {
	DefaultModel string                       `json:"default_model"`
	Providers    map[string]LLMProviderConfig `json:"providers"`
}

type LLMProviderConfig struct {
	Type                 string   `json:"type"`
	BaseURL              string   `json:"base_url,omitempty"`
	APIKey               string   `json:"api_key,omitempty"`
	Timeout              Duration `json:"timeout,omitempty"`
	FakeResponseText     string   `json:"fake_response_text,omitempty"`
	FakeToolName         string   `json:"fake_tool_name,omitempty"`
	FakeToolArgsJSON     string   `json:"fake_tool_args_json,omitempty"`
	FakeFinishReason     string   `json:"fake_finish_reason,omitempty"`
	FakeRawFinishReason  string   `json:"fake_raw_finish_reason,omitempty"`
	FakePromptTokens     int      `json:"fake_prompt_tokens,omitempty"`
	FakeCompletionTokens int      `json:"fake_completion_tokens,omitempty"`
	FakeRequestID        string   `json:"fake_request_id,omitempty"`
}

type CronJobConfig struct {
	Name           string   `json:"name"`
	ConversationID string   `json:"conversation_id"`
	ChannelType    string   `json:"channel_type"`
	ParticipantID  string   `json:"participant_id,omitempty"`
	PayloadType    string   `json:"payload_type"`
	PayloadText    string   `json:"payload_text,omitempty"`
	Interval       Duration `json:"interval"`
}

type ChannelsConfig struct {
	Telegram TelegramChannelConfig `json:"telegram,omitempty"`
}

type TelegramChannelConfig struct {
	Enabled         bool     `json:"enabled,omitempty"`
	Token           string   `json:"token,omitempty"`
	AllowedUserIDs  []int64  `json:"allowed_user_ids,omitempty"`
	LongPollTimeout Duration `json:"long_poll_timeout,omitempty"`
}

type Config struct {
	Workspace             string          `json:"workspace"`
	ListenAddr            string          `json:"listen_addr"`
	LogLevel              string          `json:"log_level"`
	APIKey                string          `json:"api_key"`
	TenantID              string          `json:"tenant_id"`
	EventQueueCapacity    int             `json:"event_queue_capacity"`
	IngestEnqueueTimeout  Duration        `json:"ingest_enqueue_timeout"`
	RateLimitTenantRPS    float64         `json:"rate_limit_tenant_rps"`
	RateLimitTenantBurst  float64         `json:"rate_limit_tenant_burst"`
	RateLimitSessionRPS   float64         `json:"rate_limit_session_rps"`
	RateLimitSessionBurst float64         `json:"rate_limit_session_burst"`
	MaxToolRounds         int             `json:"max_tool_rounds"`
	DBBusyTimeout         Duration        `json:"db_busy_timeout"`
	LLM                   LLMConfig       `json:"llm"`
	Channels              ChannelsConfig  `json:"channels,omitempty"`
	CronJobs              []CronJobConfig `json:"cron_jobs,omitempty"`
}

const (
	defaultWorkspace               = "."
	defaultListenAddr              = ":8080"
	defaultLogLevel                = "info"
	defaultTenantID                = "local"
	defaultEventQueueCapacity      = 1024
	defaultIngestEnqueueTimeout    = 200 * time.Millisecond
	defaultRateLimitTenantRPS      = 30
	defaultRateLimitTenantBurst    = 60
	defaultRateLimitSessionRPS     = 5
	defaultRateLimitSessionBurst   = 10
	defaultMaxToolRounds           = 4
	defaultDBBusyTimeout           = 5 * time.Second
	defaultProviderTimeout         = 60 * time.Second
	defaultTelegramLongPollTimeout = 30 * time.Second
	defaultFakeModel               = "fake/default"
)

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
		DBBusyTimeout:         Duration{defaultDBBusyTimeout},
		LLM: LLMConfig{
			DefaultModel: defaultFakeModel,
			Providers: map[string]LLMProviderConfig{
				"fake": {
					Type:                 "fake",
					Timeout:              Duration{defaultProviderTimeout},
					FakeResponseText:     "已收到: {{last_user_message}}",
					FakeFinishReason:     "stop",
					FakeRawFinishReason:  "stop",
					FakePromptTokens:     8,
					FakeCompletionTokens: 8,
					FakeRequestID:        "fake-request-1",
				},
			},
		},
		Channels: ChannelsConfig{
			Telegram: TelegramChannelConfig{
				LongPollTimeout: Duration{defaultTelegramLongPollTimeout},
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()
	if path == "" {
		if err := applyEnvOverrides(&cfg); err != nil {
			return cfg, err
		}
		return validate(cfg)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, err
	}
	applyDefaults(&cfg)
	if err := applyEnvOverrides(&cfg); err != nil {
		return cfg, err
	}
	return validate(cfg)
}

func applyDefaults(cfg *Config) {
	if cfg.Workspace == "" {
		cfg.Workspace = defaultWorkspace
	}
	cfg.Workspace = filepath.Clean(cfg.Workspace)
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = defaultListenAddr
	}
	if cfg.LogLevel == "" {
		cfg.LogLevel = defaultLogLevel
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
	if cfg.DBBusyTimeout.Duration <= 0 {
		cfg.DBBusyTimeout = Duration{defaultDBBusyTimeout}
	}
	if cfg.Channels.Telegram.LongPollTimeout.Duration <= 0 {
		cfg.Channels.Telegram.LongPollTimeout = Duration{defaultTelegramLongPollTimeout}
	}
	if cfg.LLM.DefaultModel == "" {
		cfg.LLM.DefaultModel = defaultFakeModel
	}
	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = map[string]LLMProviderConfig{}
	}
	for name, provider := range cfg.LLM.Providers {
		if provider.Timeout.Duration <= 0 {
			provider.Timeout = Duration{defaultProviderTimeout}
		}
		if provider.Type == "fake" {
			if provider.FakeFinishReason == "" {
				provider.FakeFinishReason = "stop"
			}
			if provider.FakeRawFinishReason == "" {
				provider.FakeRawFinishReason = provider.FakeFinishReason
			}
			if provider.FakeRequestID == "" {
				provider.FakeRequestID = "fake-request-1"
			}
			if provider.FakePromptTokens <= 0 {
				provider.FakePromptTokens = 8
			}
			if provider.FakeCompletionTokens <= 0 {
				provider.FakeCompletionTokens = 8
			}
		}
		cfg.LLM.Providers[name] = provider
	}
}

func validate(cfg Config) (Config, error) {
	applyDefaults(&cfg)
	if err := validateLogLevel(cfg.LogLevel); err != nil {
		return cfg, err
	}
	if cfg.DBBusyTimeout.Duration < time.Second {
		return cfg, errors.New("db_busy_timeout must be at least 1s")
	}
	if cfg.Channels.Telegram.Enabled && strings.TrimSpace(cfg.Channels.Telegram.Token) == "" {
		return cfg, errors.New("channels.telegram.token is required when channels.telegram.enabled=true")
	}
	providerName, _, ok := strings.Cut(cfg.LLM.DefaultModel, "/")
	if !ok || strings.TrimSpace(providerName) == "" {
		return cfg, fmt.Errorf("llm.default_model %q must use provider/model format", cfg.LLM.DefaultModel)
	}
	providerCfg, ok := cfg.LLM.Providers[providerName]
	if !ok {
		return cfg, fmt.Errorf("llm.default_model %q references unknown provider %q", cfg.LLM.DefaultModel, providerName)
	}
	switch providerCfg.Type {
	case "fake", "openai_compatible":
	default:
		return cfg, fmt.Errorf("llm.providers.%s.type %q is unsupported", providerName, providerCfg.Type)
	}
	for _, job := range cfg.CronJobs {
		if strings.TrimSpace(job.Name) == "" {
			return cfg, errors.New("cron_jobs.name is required")
		}
		if job.Interval.Duration <= 0 {
			return cfg, fmt.Errorf("cron_jobs.%s.interval must be positive", job.Name)
		}
		if strings.TrimSpace(job.ConversationID) == "" {
			return cfg, fmt.Errorf("cron_jobs.%s.conversation_id is required", job.Name)
		}
		if strings.TrimSpace(job.ChannelType) == "" {
			return cfg, fmt.Errorf("cron_jobs.%s.channel_type is required", job.Name)
		}
		if strings.TrimSpace(job.PayloadType) == "" {
			return cfg, fmt.Errorf("cron_jobs.%s.payload_type is required", job.Name)
		}
	}
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
		if len(v) >= 2 && ((v[0] == '"' && v[len(v)-1] == '"') || (v[0] == '\'' && v[len(v)-1] == '\'')) {
			v = v[1 : len(v)-1]
		}
		if k == "" {
			continue
		}
		if os.Getenv(k) == "" {
			_ = os.Setenv(k, v)
		}
	}
	return scanner.Err()
}

func applyEnvOverrides(cfg *Config) error {
	if v := strings.TrimSpace(os.Getenv("SIMICLAW_LLM_DEFAULT_MODEL")); v != "" {
		cfg.LLM.DefaultModel = v
	}
	if key := firstEnv("OPENAI_API_KEY", "LLM_API_KEY"); key != "" {
		p := cfg.LLM.Providers["openai"]
		p.Type = "openai_compatible"
		p.APIKey = key
		if p.Timeout.Duration <= 0 {
			p.Timeout = Duration{defaultProviderTimeout}
		}
		cfg.LLM.Providers["openai"] = p
	}
	if baseURL := firstEnv("OPENAI_BASE_URL", "LLM_BASE_URL"); baseURL != "" {
		p := cfg.LLM.Providers["openai"]
		p.Type = "openai_compatible"
		p.BaseURL = baseURL
		if p.Timeout.Duration <= 0 {
			p.Timeout = Duration{defaultProviderTimeout}
		}
		cfg.LLM.Providers["openai"] = p
	}
	if model := strings.TrimSpace(os.Getenv("LLM_MODEL")); model != "" {
		cfg.LLM.DefaultModel = model
	}
	if enabled := strings.TrimSpace(os.Getenv("TELEGRAM_ENABLED")); enabled != "" {
		parsed, err := strconv.ParseBool(enabled)
		if err != nil {
			return fmt.Errorf("invalid TELEGRAM_ENABLED %q: %w", enabled, err)
		}
		cfg.Channels.Telegram.Enabled = parsed
	}
	if token := strings.TrimSpace(os.Getenv("TELEGRAM_TOKEN")); token != "" {
		cfg.Channels.Telegram.Token = token
	}
	if allowed := strings.TrimSpace(os.Getenv("TELEGRAM_ALLOWED_USER_IDS")); allowed != "" {
		parsed, err := parseTelegramAllowedUserIDs(allowed)
		if err != nil {
			return err
		}
		cfg.Channels.Telegram.AllowedUserIDs = parsed
	}
	if rawTimeout := strings.TrimSpace(os.Getenv("TELEGRAM_LONG_POLL_TIMEOUT")); rawTimeout != "" {
		parsed, err := time.ParseDuration(rawTimeout)
		if err != nil {
			return fmt.Errorf("invalid TELEGRAM_LONG_POLL_TIMEOUT %q: %w", rawTimeout, err)
		}
		cfg.Channels.Telegram.LongPollTimeout = Duration{parsed}
	}
	return nil
}

func parseTelegramAllowedUserIDs(raw string) ([]int64, error) {
	parts := strings.Split(raw, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		id, err := strconv.ParseInt(trimmed, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid TELEGRAM_ALLOWED_USER_IDS entry %q: %w", trimmed, err)
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func firstEnv(keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return value
		}
	}
	return ""
}
