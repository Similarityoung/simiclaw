package prompt

import (
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
