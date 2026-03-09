package initcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/internal/ui/messages"
)

func TestEmbeddedWorkspaceTemplatesContainExpectedFiles(t *testing.T) {
	got := sortedWorkspaceTemplateNames()
	want := []string{"BOOTSTRAP.md", "HEARTBEAT.md", "IDENTITY.md", "SOUL.md", "TOOLS.md", "USER.md"}
	if len(got) != len(want) {
		t.Fatalf("unexpected template count got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected template order got=%v want=%v", got, want)
		}
	}
	if !strings.Contains(workspaceTemplates["BOOTSTRAP.md"], "Please delete this file manually") {
		t.Fatalf("expected embedded bootstrap template warning, got %q", workspaceTemplates["BOOTSTRAP.md"])
	}
}

func TestRunScaffoldsLayeredPromptFiles(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	var out bytes.Buffer

	if err := run(Options{Workspace: workspace}, common.IOStreams{Out: &out}); err != nil {
		t.Fatalf("run init: %v", err)
	}
	if out.String() != messages.WorkspaceInitialized(workspace) {
		t.Fatalf("expected init output %q, got %q", messages.WorkspaceInitialized(workspace), out.String())
	}

	for _, name := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "TOOLS.md", "BOOTSTRAP.md", "HEARTBEAT.md"} {
		path := filepath.Join(workspace, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(workspace, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to remain absent, err=%v", err)
	}

	bootstrap, err := os.ReadFile(filepath.Join(workspace, "BOOTSTRAP.md"))
	if err != nil {
		t.Fatalf("read BOOTSTRAP.md: %v", err)
	}
	if !strings.Contains(string(bootstrap), "Please delete this file manually") {
		t.Fatalf("expected bootstrap warning, got %q", string(bootstrap))
	}
}

func TestRunDoesNotOverwriteExistingPromptFiles(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(workspace, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	userPath := filepath.Join(workspace, "USER.md")
	if err := os.WriteFile(userPath, []byte("custom user prefs\n"), 0o644); err != nil {
		t.Fatalf("seed USER.md: %v", err)
	}

	if err := run(Options{Workspace: workspace}, common.IOStreams{}); err != nil {
		t.Fatalf("run init: %v", err)
	}

	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if string(data) != "custom user prefs\n" {
		t.Fatalf("expected existing USER.md to be preserved, got %q", string(data))
	}
}
