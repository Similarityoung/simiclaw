package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"google.golang.org/adk/agent"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/mcptoolset"
)

type fakeToolset struct {
	name string
}

func (f fakeToolset) Name() string {
	return f.name
}

func (f fakeToolset) Tools(agent.ReadonlyContext) ([]adktool.Tool, error) {
	return nil, nil
}

func TestLoadDynamicMCPToolsetsMissingConfigIsNonFatal(t *testing.T) {
	workspace := t.TempDir()
	toolsets, err := loadDynamicMCPToolsets(workspace, func(cfg mcptoolset.Config) (adktool.Toolset, error) {
		t.Fatalf("expected no toolset initialization when plugins.json is missing")
		return nil, nil
	})
	if err != nil {
		t.Fatalf("expected missing plugins.json to be non-fatal, got error: %v", err)
	}
	if len(toolsets) != 0 {
		t.Fatalf("expected zero toolsets when plugins.json is missing, got: %d", len(toolsets))
	}
}

func TestLoadDynamicMCPToolsetsReturnsErrorForInvalidConfig(t *testing.T) {
	workspace := t.TempDir()
	configPath := filepath.Join(workspace, pluginsConfigFileName)
	if err := os.WriteFile(configPath, []byte("{"), 0o644); err != nil {
		t.Fatalf("write plugins.json: %v", err)
	}

	_, err := loadDynamicMCPToolsets(workspace, func(cfg mcptoolset.Config) (adktool.Toolset, error) {
		return fakeToolset{name: "unused"}, nil
	})
	if err == nil {
		t.Fatalf("expected parse error for invalid plugins.json")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Fatalf("expected parse error prefix, got: %v", err)
	}
}

func TestLoadDynamicMCPToolsetsFiltersByEnableTrustAndAllowlist(t *testing.T) {
	workspace := t.TempDir()
	cfg := Config{
		Allowlist: []string{"trusted-enabled"},
		Plugins: []PluginEntry{
			{Name: "disabled", Enabled: false, Trusted: true, Command: "echo"},
			{Name: "untrusted", Enabled: true, Trusted: false, Command: "echo"},
			{Name: "not-allowlisted", Enabled: true, Trusted: true, Command: "echo"},
			{Name: "trusted-enabled", Enabled: true, Trusted: true, Command: "echo"},
		},
	}
	writePluginsConfig(t, workspace, cfg)

	var calls int
	toolsets, err := loadDynamicMCPToolsets(workspace, func(cfg mcptoolset.Config) (adktool.Toolset, error) {
		calls++
		if cfg.Transport == nil {
			t.Fatalf("expected transport to be initialized")
		}
		return fakeToolset{name: "trusted-enabled"}, nil
	})
	if err != nil {
		t.Fatalf("expected plugin load success, got error: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected exactly one trusted+allowlisted plugin initialization, got: %d", calls)
	}
	if len(toolsets) != 1 {
		t.Fatalf("expected exactly one toolset, got: %d", len(toolsets))
	}
}

func TestLoadDynamicMCPToolsetsReturnsErrorForInvalidTrustedPlugin(t *testing.T) {
	workspace := t.TempDir()
	cfg := Config{
		Allowlist: []string{"broken"},
		Plugins: []PluginEntry{
			{Name: "broken", Enabled: true, Trusted: true, Transport: "command"},
		},
	}
	writePluginsConfig(t, workspace, cfg)

	_, err := loadDynamicMCPToolsets(workspace, func(cfg mcptoolset.Config) (adktool.Toolset, error) {
		return fakeToolset{name: "unused"}, nil
	})
	if err == nil {
		t.Fatalf("expected validation error for invalid trusted plugin")
	}
	if !strings.Contains(err.Error(), "requires command") {
		t.Fatalf("expected command validation error, got: %v", err)
	}
}

func writePluginsConfig(t *testing.T, workspace string, cfg Config) {
	t.Helper()
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal plugins config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, pluginsConfigFileName), b, 0o644); err != nil {
		t.Fatalf("write plugins config: %v", err)
	}
}
