package session

import (
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestComputeKeyDMThreadIgnored(t *testing.T) {
	convA := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "thread_a"}
	convB := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "thread_b"}

	k1, err := ComputeKey("local", convA, "default")
	if err != nil {
		t.Fatalf("ComputeKey error: %v", err)
	}
	k2, err := ComputeKey("local", convB, "default")
	if err != nil {
		t.Fatalf("ComputeKey error: %v", err)
	}
	if k1 != k2 {
		t.Fatalf("expected same session key, got %q and %q", k1, k2)
	}
}

func TestComputeKeyDMRequiresParticipant(t *testing.T) {
	_, err := ComputeKey("local", model.Conversation{ConversationID: "conv_1", ChannelType: "dm"}, "default")
	if err == nil {
		t.Fatalf("expected error for missing participant_id")
	}
}

func TestIsNewSessionCommand(t *testing.T) {
	for _, input := range []string{"/new", "  /new  ", "/new@simiclaw_bot"} {
		if !IsNewSessionCommand(input) {
			t.Fatalf("expected %q to be treated as /new", input)
		}
	}
	for _, input := range []string{"/new later", "hello /new", "/news"} {
		if IsNewSessionCommand(input) {
			t.Fatalf("expected %q not to be treated as /new", input)
		}
	}
}

func TestNewScopeFromIDStable(t *testing.T) {
	a := NewScopeFromID("telegram:update:123")
	b := NewScopeFromID("telegram:update:123")
	c := NewScopeFromID("telegram:update:124")
	if a != b {
		t.Fatalf("expected stable scope, got %q and %q", a, b)
	}
	if a == c {
		t.Fatalf("expected different scope ids, got %q and %q", a, c)
	}
}
