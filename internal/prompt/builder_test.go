package prompt

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestBuilderBuildIncludesFiveSectionsInOrder(t *testing.T) {
	b := NewBuilder(t.TempDir())
	got := b.Build(BuildInput{Context: RunContext{
		Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC),
		Conversation: model.Conversation{
			ConversationID: "conv-1",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		SessionKey:  "tenant:dm:u1",
		SessionID:   "ses_1",
		PayloadType: "message",
	}})

	sections := []string{
		"## Identity & Runtime Rules",
		"## Project Context",
		"## Available Skills",
		"## Memory Policy",
		"## Current Run Context",
	}
	last := -1
	for _, section := range sections {
		idx := strings.Index(got, section)
		if idx < 0 {
			t.Fatalf("missing section %q in prompt: %s", section, got)
		}
		if idx <= last {
			t.Fatalf("section %q out of order in prompt: %s", section, got)
		}
		last = idx
	}
	if !strings.Contains(got, "2026-03-08T09:10:11Z") {
		t.Fatalf("expected UTC timestamp in prompt, got: %s", got)
	}
}

func TestBuilderInjectsBootstrapFilesInOrder(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "AGENTS.md"), "agents rules")
	writeFile(t, filepath.Join(workspace, "IDENTITY.md"), "identity profile")
	writeFile(t, filepath.Join(workspace, "USER.md"), "user prefs")

	b := NewBuilder(workspace)
	got := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})

	wantOrder := []string{"### AGENTS.md", "agents rules", "### IDENTITY.md", "identity profile", "### USER.md", "user prefs"}
	last := -1
	for _, needle := range wantOrder {
		idx := strings.Index(got, needle)
		if idx < 0 {
			t.Fatalf("missing injected content %q in prompt: %s", needle, got)
		}
		if idx <= last {
			t.Fatalf("injected content %q out of order in prompt: %s", needle, got)
		}
		last = idx
	}
}

func TestBuilderSkipsMissingBootstrapFiles(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "USER.md"), "user prefs")

	b := NewBuilder(workspace)
	got := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})

	if strings.Contains(got, "### AGENTS.md") || strings.Contains(got, "### IDENTITY.md") {
		t.Fatalf("expected missing bootstrap files to be skipped, got: %s", got)
	}
	if !strings.Contains(got, "### USER.md") {
		t.Fatalf("expected USER.md to be injected, got: %s", got)
	}
}

func TestBuilderReusesCacheAndInvalidatesOnBootstrapChange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "AGENTS.md")
	writeFile(t, path, "v1")

	b := NewBuilder(workspace)
	first := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})
	second := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 12, 0, time.UTC)}})
	if b.staticBuilds != 1 {
		t.Fatalf("expected cached static prefix to be reused, got staticBuilds=%d", b.staticBuilds)
	}
	if !strings.Contains(first, "v1") || !strings.Contains(second, "v1") {
		t.Fatalf("expected cached content to contain v1, first=%q second=%q", first, second)
	}

	writeFile(t, path, "v2")
	future := time.Date(2026, 3, 8, 9, 10, 30, 0, time.UTC)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes bootstrap file: %v", err)
	}
	third := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 13, 0, time.UTC)}})
	if b.staticBuilds != 2 {
		t.Fatalf("expected cache invalidation after bootstrap change, got staticBuilds=%d", b.staticBuilds)
	}
	if !strings.Contains(third, "v2") {
		t.Fatalf("expected updated bootstrap content after invalidation, got: %s", third)
	}
}

func TestBuilderInjectsSortedSkillSummary(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "skills", "beta", "SKILL.md"), "---\nname: Beta\ndescription: second skill\n---\n\n# Beta")
	writeFile(t, filepath.Join(workspace, "skills", "alpha", "SKILL.md"), "---\nname: Alpha\ndescription: first skill\n---\n\n# Alpha")

	b := NewBuilder(workspace)
	got := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})

	alpha := strings.Index(got, "- Alpha — first skill (skills/alpha/SKILL.md)")
	beta := strings.Index(got, "- Beta — second skill (skills/beta/SKILL.md)")
	if alpha < 0 || beta < 0 {
		t.Fatalf("expected sorted skill summary in prompt, got: %s", got)
	}
	if alpha >= beta {
		t.Fatalf("expected Alpha before Beta, got: %s", got)
	}
	if !strings.Contains(got, "context_get") {
		t.Fatalf("expected prompt to mention context_get, got: %s", got)
	}
}

func TestBuilderSkillSummaryFallsBackWithoutFrontmatter(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "skills", "fallback", "SKILL.md"), "# Fallback title\n\nUse this skill when fallback is needed.")

	b := NewBuilder(workspace)
	got := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})

	if !strings.Contains(got, "- fallback — Fallback title (skills/fallback/SKILL.md)") {
		t.Fatalf("expected fallback skill summary, got: %s", got)
	}
}

func TestBuilderSkipsInvalidSkillFile(t *testing.T) {
	workspace := t.TempDir()
	writeFile(t, filepath.Join(workspace, "skills", "good", "SKILL.md"), "---\nname: Good\ndescription: safe\n---\n\n# Good")
	writeFile(t, filepath.Join(workspace, "skills", "bad", "SKILL.md"), "---\nname Good\n---\n\n# Broken")

	b := NewBuilder(workspace)
	got := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})

	if !strings.Contains(got, "- Good — safe (skills/good/SKILL.md)") {
		t.Fatalf("expected valid skill to remain, got: %s", got)
	}
	if strings.Contains(got, "skills/bad/SKILL.md") || strings.Contains(got, "Broken") {
		t.Fatalf("expected invalid skill to be skipped, got: %s", got)
	}
}

func TestBuilderInvalidatesCacheOnSkillChange(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "skills", "alpha", "SKILL.md")
	writeFile(t, path, "---\nname: Alpha\ndescription: v1\n---\n\n# Alpha")

	b := NewBuilder(workspace)
	first := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 11, 0, time.UTC)}})
	second := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 12, 0, time.UTC)}})
	if b.staticBuilds != 1 {
		t.Fatalf("expected cached static prefix to be reused before skill change, got=%d", b.staticBuilds)
	}
	if !strings.Contains(first, "v1") || !strings.Contains(second, "v1") {
		t.Fatalf("expected v1 skill summary before invalidation, first=%q second=%q", first, second)
	}

	writeFile(t, path, "---\nname: Alpha\ndescription: v2\n---\n\n# Alpha")
	future := time.Date(2026, 3, 8, 9, 11, 0, 0, time.UTC)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes skill file: %v", err)
	}
	third := b.Build(BuildInput{Context: RunContext{Now: time.Date(2026, 3, 8, 9, 10, 13, 0, time.UTC)}})
	if b.staticBuilds != 2 {
		t.Fatalf("expected skill change to invalidate cache, got=%d", b.staticBuilds)
	}
	if !strings.Contains(third, "v2") {
		t.Fatalf("expected v2 skill summary after invalidation, got: %s", third)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
