package initcmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
)

func TestRunScaffoldsLayeredPromptFiles(t *testing.T) {
	workspace := filepath.Join(t.TempDir(), "workspace")
	var out bytes.Buffer

	if err := run(Options{Workspace: workspace}, common.IOStreams{Out: &out}); err != nil {
		t.Fatalf("run init: %v", err)
	}
	if !strings.Contains(out.String(), "workspace initialized at") {
		t.Fatalf("expected init output, got %q", out.String())
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
	if !strings.Contains(string(bootstrap), "请手动删除本文件") {
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
