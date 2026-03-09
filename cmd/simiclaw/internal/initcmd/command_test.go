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
	if !strings.Contains(string(bootstrap), "If file-write tools are unavailable") {
		t.Fatalf("expected bootstrap warning, got %q", string(bootstrap))
	}
}
