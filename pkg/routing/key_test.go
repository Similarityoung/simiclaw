package routing

import (
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestComputeKey_DM_ThreadIgnored(t *testing.T) {
	convA := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "th_1"}
	convB := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "th_2"}

	k1, err := ComputeKey("local", convA, "default")
	if err != nil {
		t.Fatalf("ComputeKey error: %v", err)
	}
	k2, err := ComputeKey("local", convB, "default")
	if err != nil {
		t.Fatalf("ComputeKey error: %v", err)
	}
	if k1 != k2 {
		t.Fatalf("expected same key when only thread_id differs, got %s != %s", k1, k2)
	}
}

func TestComputeKey_DMRequiresParticipant(t *testing.T) {
	_, err := ComputeKey("local", model.Conversation{ConversationID: "conv_1", ChannelType: "dm"}, "default")
	if err == nil {
		t.Fatalf("expected error for missing participant_id")
	}
}
