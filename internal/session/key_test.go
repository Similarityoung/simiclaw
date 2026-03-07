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
