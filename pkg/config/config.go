package config

import internalconfig "github.com/similarityyoung/simiclaw/internal/config"

type Duration = internalconfig.Duration
type LLMConfig = internalconfig.LLMConfig
type LLMProviderConfig = internalconfig.LLMProviderConfig
type CronJobConfig = internalconfig.CronJobConfig
type ChannelsConfig = internalconfig.ChannelsConfig
type TelegramChannelConfig = internalconfig.TelegramChannelConfig
type WebSearchConfig = internalconfig.WebSearchConfig
type Config = internalconfig.Config

func Default() Config {
	return internalconfig.Default()
}

func Load(path string) (Config, error) {
	return internalconfig.Load(path)
}

func LoadDotEnv(path string) error {
	return internalconfig.LoadDotEnv(path)
}
