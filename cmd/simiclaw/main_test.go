package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/config"
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

func TestRunWithoutArgsShowsHelpInNonInteractiveMode(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(nil, common.IOStreams{Out: &stdout, ErrOut: &stderr})
	if code != 0 {
		t.Fatalf("run() code=%d want=0 stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "SimiClaw CLI v2") {
		t.Fatalf("expected help output, got %q", stdout.String())
	}
}

func TestRunReturnsUsageExitCodeForUnknownCommand(t *testing.T) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run([]string{"unknown-command"}, common.IOStreams{Out: &stdout, ErrOut: &stderr})
	if code != 2 {
		t.Fatalf("run() code=%d want=2 stderr=%q", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("expected unknown command error, got %q", stderr.String())
	}
}
