package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbeddedTemplatesContainExpectedFiles(t *testing.T) {
	got := templateNames()
	want := []string{"BOOTSTRAP.md", "HEARTBEAT.md", "IDENTITY.md", "SOUL.md", "TOOLS.md", "USER.md"}
	if len(got) != len(want) {
		t.Fatalf("unexpected template count got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected template order got=%v want=%v", got, want)
		}
	}
	if !strings.Contains(templates["BOOTSTRAP.md"], "Please delete this file manually") {
		t.Fatalf("expected embedded bootstrap template warning, got %q", templates["BOOTSTRAP.md"])
	}
}

func TestScaffoldFilesCreatesMissingTemplates(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := ScaffoldFiles(root); err != nil {
		t.Fatalf("ScaffoldFiles: %v", err)
	}
	for _, name := range []string{"SOUL.md", "IDENTITY.md", "USER.md", "TOOLS.md", "BOOTSTRAP.md", "HEARTBEAT.md"} {
		path := filepath.Join(root, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected %s to exist: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "AGENTS.md")); !os.IsNotExist(err) {
		t.Fatalf("expected AGENTS.md to remain absent, err=%v", err)
	}
}

func TestScaffoldFilesDoesNotOverwriteExistingFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	userPath := filepath.Join(root, "USER.md")
	if err := os.WriteFile(userPath, []byte("custom user prefs\n"), 0o644); err != nil {
		t.Fatalf("seed USER.md: %v", err)
	}

	if err := ScaffoldFiles(root); err != nil {
		t.Fatalf("ScaffoldFiles: %v", err)
	}

	data, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("read USER.md: %v", err)
	}
	if string(data) != "custom user prefs\n" {
		t.Fatalf("expected existing USER.md to be preserved, got %q", string(data))
	}
}
