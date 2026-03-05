package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssembleInstructionInjectionReturnsEmptyWhenSkillsDirMissing(t *testing.T) {
	workspace := t.TempDir()

	injection, err := AssembleInstructionInjection(workspace)
	if err != nil {
		t.Fatalf("expected missing skills dir to be non-fatal, got error: %v", err)
	}
	if injection != "" {
		t.Fatalf("expected empty injection when skills directory is missing, got: %q", injection)
	}
}

func TestAssembleInstructionInjectionUsesDeterministicOrder(t *testing.T) {
	workspace := t.TempDir()

	writeSkillFile(t, workspace, "b-team/SKILL.md", "B skill")
	writeSkillFile(t, workspace, "a-team/nested/SKILL.md", "A nested skill")
	writeSkillFile(t, workspace, "a-team/SKILL.md", "A root skill")
	writeSkillFile(t, workspace, "a-team/IGNORE.md", "ignored")

	injection, err := AssembleInstructionInjection(workspace)
	if err != nil {
		t.Fatalf("expected injection assembly to succeed, got error: %v", err)
	}

	first := strings.Index(injection, "### a-team/SKILL.md")
	second := strings.Index(injection, "### a-team/nested/SKILL.md")
	third := strings.Index(injection, "### b-team/SKILL.md")
	if first == -1 || second == -1 || third == -1 {
		t.Fatalf("expected all skill files to be present in injection, got: %s", injection)
	}
	if !(first < second && second < third) {
		t.Fatalf("expected deterministic lexical ordering, got: %s", injection)
	}
	if strings.Contains(injection, "IGNORE.md") {
		t.Fatalf("expected non SKILL.md files to be ignored, got: %s", injection)
	}
}

func TestAssembleInstructionInjectionIncludesContent(t *testing.T) {
	workspace := t.TempDir()
	writeSkillFile(t, workspace, "ops/SKILL.md", "Always confirm maintenance window.")

	injection, err := AssembleInstructionInjection(workspace)
	if err != nil {
		t.Fatalf("expected injection assembly to succeed, got error: %v", err)
	}
	if !strings.Contains(injection, SkillsInjectionHeader) {
		t.Fatalf("expected injection header, got: %s", injection)
	}
	if !strings.Contains(injection, "Always confirm maintenance window.") {
		t.Fatalf("expected skill content in injection, got: %s", injection)
	}
}

func writeSkillFile(t *testing.T, workspace, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(workspace, "skills", filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("create skill parent directory: %v", err)
	}
	if err := os.WriteFile(absPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill file: %v", err)
	}
}
