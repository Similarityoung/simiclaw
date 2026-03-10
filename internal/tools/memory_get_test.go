package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestMemoryGetRejectsPrivateInGroup(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "secret.md"), []byte("secret=42\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{
		Workspace:    workspace,
		Conversation: model.Conversation{ChannelType: "group"},
	}, "memory_get", map[string]any{
		"path":  "memory/private/secret.md",
		"lines": []int{1, 1},
	})

	if res.Error == nil {
		t.Fatalf("expected forbidden error")
	}
	if res.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN, got %+v", res.Error)
	}
}

func TestMemoryGetAllowsPrivateInDM(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "secret.md"), []byte("secret=42\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{
		Workspace:    workspace,
		Conversation: model.Conversation{ChannelType: "dm"},
	}, "memory_get", map[string]any{
		"path":  "memory/private/secret.md",
		"lines": []int{1, 1},
	})

	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	content, _ := res.Output["content"].(string)
	if !strings.Contains(content, "secret=42") {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestMemoryGetRejectsPublicSymlinkToPrivateInGroup(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "private"), 0o755); err != nil {
		t.Fatalf("mkdir private: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "private", "secret.md"), []byte("secret=42\n"), 0o644); err != nil {
		t.Fatalf("write private: %v", err)
	}
	if err := os.Symlink(filepath.Join("..", "private", "secret.md"), filepath.Join(workspace, "memory", "public", "alias.md")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{
		Workspace:    workspace,
		Conversation: model.Conversation{ChannelType: "group"},
	}, "memory_get", map[string]any{
		"path":  "memory/public/alias.md",
		"lines": []int{1, 1},
	})

	if res.Error == nil {
		t.Fatalf("expected forbidden error")
	}
	if res.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN, got %+v", res.Error)
	}
}

func TestMemorySearchDisabledMatchesOutput(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "memory", "public"), 0o755); err != nil {
		t.Fatalf("mkdir public: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "memory", "public", "note.md"), []byte("query-hit\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{
		Workspace:    workspace,
		Conversation: model.Conversation{ChannelType: "group"},
	}, "memory_search", map[string]any{
		"query":      "query-hit",
		"visibility": "auto",
		"kind":       "any",
		"top_k":      1,
	})

	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	disabled, ok := res.Output["disabled"].(bool)
	if !ok {
		t.Fatalf("disabled output should be bool, got=%T", res.Output["disabled"])
	}
	if res.Disabled != disabled {
		t.Fatalf("disabled mismatch: struct=%v output=%v", res.Disabled, disabled)
	}
}
