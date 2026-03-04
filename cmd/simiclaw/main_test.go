package main

import (
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/config"
)

func TestResolveBootstrapLogLevelUsesEnv(t *testing.T) {
	t.Setenv(bootstrapLogLevelEnv, "debug")
	if got := resolveBootstrapLogLevel(); got != "debug" {
		t.Fatalf("resolveBootstrapLogLevel()=%q want=%q", got, "debug")
	}
}

func TestResolveBootstrapLogLevelUsesDefaultWhenEnvEmpty(t *testing.T) {
	t.Setenv(bootstrapLogLevelEnv, "   ")
	want := config.Default().LogLevel
	if got := resolveBootstrapLogLevel(); got != want {
		t.Fatalf("resolveBootstrapLogLevel()=%q want=%q", got, want)
	}
}
