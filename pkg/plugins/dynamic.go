package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/adk/tool"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/mcptoolset"
)

const pluginsConfigFileName = "plugins.json"

// Config represents workspace/plugins.json.
//
// Expected format:
//
//	{
//	  "allowlist": ["filesystem", "github"],
//	  "plugins": [
//	    {
//	      "name": "filesystem",
//	      "enabled": true,
//	      "trusted": true,
//	      "transport": "command",
//	      "command": "npx",
//	      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"],
//	      "allowed_tools": ["read_file", "list_directory"]
//	    }
//	  ]
//	}
type Config struct {
	Allowlist []string      `json:"allowlist"`
	Plugins   []PluginEntry `json:"plugins"`
}

type PluginEntry struct {
	Name                string   `json:"name"`
	Enabled             bool     `json:"enabled"`
	Trusted             bool     `json:"trusted"`
	Transport           string   `json:"transport,omitempty"`
	Command             string   `json:"command,omitempty"`
	Args                []string `json:"args,omitempty"`
	URL                 string   `json:"url,omitempty"`
	AllowedTools        []string `json:"allowed_tools,omitempty"`
	RequireConfirmation bool     `json:"require_confirmation,omitempty"`
}

func LoadDynamicMCPToolsets(workspace string) ([]adktool.Toolset, error) {
	return loadDynamicMCPToolsets(workspace, mcptoolset.New)
}

func loadDynamicMCPToolsets(workspace string, newToolset func(mcptoolset.Config) (adktool.Toolset, error)) ([]adktool.Toolset, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("workspace is required")
	}

	cfgPath := filepath.Join(workspace, pluginsConfigFileName)
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", cfgPath, err)
	}

	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", cfgPath, err)
	}

	allow := map[string]struct{}{}
	for _, raw := range cfg.Allowlist {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		allow[name] = struct{}{}
	}

	toolsets := make([]adktool.Toolset, 0, len(cfg.Plugins))
	for i := range cfg.Plugins {
		entry := cfg.Plugins[i]
		if !entry.Enabled || !entry.Trusted {
			continue
		}
		if _, ok := allow[strings.TrimSpace(entry.Name)]; !ok {
			continue
		}

		transport, err := buildTransport(entry)
		if err != nil {
			return nil, fmt.Errorf("plugin %q: %w", entry.Name, err)
		}

		ts, err := newToolset(mcptoolset.Config{
			Transport:           transport,
			RequireConfirmation: entry.RequireConfirmation,
		})
		if err != nil {
			return nil, fmt.Errorf("plugin %q: initialize mcp toolset: %w", entry.Name, err)
		}

		if len(entry.AllowedTools) > 0 {
			ts = tool.FilterToolset(ts, tool.StringPredicate(entry.AllowedTools))
		}

		toolsets = append(toolsets, ts)
	}

	return toolsets, nil
}

func buildTransport(entry PluginEntry) (mcp.Transport, error) {
	name := strings.TrimSpace(entry.Name)
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	transport := strings.TrimSpace(entry.Transport)
	if transport == "" {
		transport = "command"
	}

	switch transport {
	case "command":
		command := strings.TrimSpace(entry.Command)
		if command == "" {
			return nil, fmt.Errorf("command transport requires command")
		}
		return &mcp.CommandTransport{Command: exec.Command(command, entry.Args...)}, nil
	case "sse":
		url := strings.TrimSpace(entry.URL)
		if url == "" {
			return nil, fmt.Errorf("sse transport requires url")
		}
		return &mcp.SSEClientTransport{Endpoint: url}, nil
	default:
		return nil, fmt.Errorf("unsupported transport %q", transport)
	}
}
