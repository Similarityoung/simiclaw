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

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}
