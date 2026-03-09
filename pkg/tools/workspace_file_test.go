package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestWorkspacePatchReplacesExactMatch(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "IDENTITY.md"), "- Name: SimiClaw\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "IDENTITY.md",
		"old_text": "SimiClaw",
		"new_text": "Simi 龙虾",
	})

	if res.Error != nil {
		fatalToolError(t, res.Error)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "IDENTITY.md"))
	if err != nil {
		t.Fatalf("read IDENTITY.md: %v", err)
	}
	if string(data) != "- Name: Simi 龙虾\n" {
		t.Fatalf("unexpected patched content: %q", string(data))
	}
	if res.Output["operation"] != "patched" {
		t.Fatalf("unexpected output: %+v", res.Output)
	}
	if _, ok := res.Output["sha256"].(string); !ok {
		t.Fatalf("expected sha256 output, got %+v", res.Output)
	}
	bytesWritten, ok := res.Output["bytes_written"].(int)
	if !ok || bytesWritten <= 0 {
		t.Fatalf("expected positive bytes_written, got %+v", res.Output)
	}
}

func TestWorkspacePatchRejectsZeroAndMultiMatch(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "USER.md"), "alpha\nbeta\nalpha\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)

	zero := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "USER.md",
		"old_text": "gamma",
		"new_text": "delta",
	})
	if zero.Error == nil || zero.Error.Code != model.ErrorCodeConflict {
		t.Fatalf("expected CONFLICT for zero match, got %+v", zero.Error)
	}
	if zero.Error.Details["match_count"] != 0 {
		t.Fatalf("expected match_count=0, got %+v", zero.Error)
	}

	multi := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "USER.md",
		"old_text": "alpha",
		"new_text": "delta",
	})
	if multi.Error == nil || multi.Error.Code != model.ErrorCodeConflict {
		t.Fatalf("expected CONFLICT for multi match, got %+v", multi.Error)
	}
	if multi.Error.Details["match_count"] != 2 {
		t.Fatalf("expected match_count=2, got %+v", multi.Error)
	}
}

func TestWorkspacePatchCreatesNewTextFile(t *testing.T) {
	workspace := t.TempDir()

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "notes/today.md",
		"new_text": "hello\nworld",
		"create":   true,
	})

	if res.Error != nil {
		fatalToolError(t, res.Error)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "notes", "today.md"))
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	if string(data) != "hello\nworld" {
		t.Fatalf("unexpected created content: %q", string(data))
	}
	if res.Output["operation"] != "created" {
		t.Fatalf("unexpected output: %+v", res.Output)
	}
}

func TestWorkspacePatchCreateExistingReturnsConflict(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "AGENTS.md"), "rules\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "AGENTS.md",
		"new_text": "new rules",
		"create":   true,
	})

	if res.Error == nil || res.Error.Code != model.ErrorCodeConflict {
		t.Fatalf("expected CONFLICT, got %+v", res.Error)
	}
}

func TestWorkspaceDeleteRemovesFile(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "BOOTSTRAP.md"), "bootstrap\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)
	res := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_delete", map[string]any{
		"path": "BOOTSTRAP.md",
	})

	if res.Error != nil {
		fatalToolError(t, res.Error)
	}
	if _, err := os.Stat(filepath.Join(workspace, "BOOTSTRAP.md")); !os.IsNotExist(err) {
		t.Fatalf("expected file deleted, err=%v", err)
	}
	if res.Output["operation"] != "deleted" {
		t.Fatalf("unexpected output: %+v", res.Output)
	}
}

func TestWorkspaceDeleteReturnsNotFoundAndRejectsDirectory(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)
	notFound := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_delete", map[string]any{
		"path": "missing.md",
	})
	if notFound.Error == nil || notFound.Error.Code != model.ErrorCodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %+v", notFound.Error)
	}

	dir := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_delete", map[string]any{
		"path": "notes",
	})
	if dir.Error == nil || dir.Error.Code != model.ErrorCodeInvalidArgument {
		t.Fatalf("expected INVALID_ARGUMENT, got %+v", dir.Error)
	}
}

func TestWorkspaceToolsRejectEscapeRuntimeAndSymlinkEscape(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "runtime"), 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	writeContextFile(t, filepath.Join(workspace, "runtime", "blocked.txt"), "nope\n")
	outside := t.TempDir()
	writeContextFile(t, filepath.Join(outside, "secret.txt"), "top secret\n")
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(workspace, "escape.txt")); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	reg := NewRegistry()
	RegisterBuiltins(reg)

	for _, tc := range []struct {
		name string
		tool string
		args map[string]any
	}{
		{name: "runtime", tool: "workspace_patch", args: map[string]any{"path": "runtime/blocked.txt", "old_text": "nope", "new_text": "yes"}},
		{name: "parent escape", tool: "workspace_patch", args: map[string]any{"path": "../oops.txt", "new_text": "x", "create": true}},
		{name: "symlink escape", tool: "workspace_patch", args: map[string]any{"path": "escape.txt", "old_text": "top", "new_text": "low"}},
	} {
		res := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, tc.tool, tc.args)
		if res.Error == nil || res.Error.Code != model.ErrorCodeForbidden {
			t.Fatalf("%s expected FORBIDDEN, got %+v", tc.name, res.Error)
		}
	}
}

func TestWorkspaceToolsPrivateMemoryRequiresDM(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "memory", "private", "prefs.md"), "secret=42\n")

	reg := NewRegistry()
	RegisterBuiltins(reg)

	groupPatch := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "group"}}, "workspace_patch", map[string]any{
		"path":     "memory/private/prefs.md",
		"old_text": "42",
		"new_text": "43",
	})
	if groupPatch.Error == nil || groupPatch.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected group patch FORBIDDEN, got %+v", groupPatch.Error)
	}

	groupDelete := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "group"}}, "workspace_delete", map[string]any{
		"path": "memory/private/prefs.md",
	})
	if groupDelete.Error == nil || groupDelete.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected group delete FORBIDDEN, got %+v", groupDelete.Error)
	}

	dmPatch := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "memory/private/prefs.md",
		"old_text": "42",
		"new_text": "43",
	})
	if dmPatch.Error != nil {
		fatalToolError(t, dmPatch.Error)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "memory", "private", "prefs.md"))
	if err != nil {
		t.Fatalf("read prefs: %v", err)
	}
	if !strings.Contains(string(data), "43") {
		t.Fatalf("unexpected dm patched content: %q", string(data))
	}
}

func TestWorkspaceToolsRejectNonTextAndTooLargeContent(t *testing.T) {
	workspace := t.TempDir()
	writeContextFile(t, filepath.Join(workspace, "binary.txt"), string([]byte{'a', 0, 'b'}))
	tooLarge := strings.Repeat("x", 256*1024+1)

	reg := NewRegistry()
	RegisterBuiltins(reg)

	binaryRes := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "binary.txt",
		"old_text": "a",
		"new_text": "b",
	})
	if binaryRes.Error == nil || binaryRes.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN for non-text file, got %+v", binaryRes.Error)
	}

	largeCreate := reg.Call(context.Background(), Context{Workspace: workspace, Conversation: model.Conversation{ChannelType: "dm"}}, "workspace_patch", map[string]any{
		"path":     "large.txt",
		"new_text": tooLarge,
		"create":   true,
	})
	if largeCreate.Error == nil || largeCreate.Error.Code != model.ErrorCodeForbidden {
		t.Fatalf("expected FORBIDDEN for oversized create, got %+v", largeCreate.Error)
	}
}

func fatalToolError(t *testing.T, err *model.ErrorBlock) {
	t.Helper()
	t.Fatalf("unexpected tool error: %+v", err)
}
