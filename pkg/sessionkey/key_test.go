package sessionkey

import (
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestComputeSessionKey_DM_ThreadIgnored(t *testing.T) {
	convA := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "th_1"}
	convB := model.Conversation{ConversationID: "conv_1", ChannelType: "dm", ParticipantID: "u1", ThreadID: "th_2"}

	k1, err := ComputeSessionKey("local", convA, "default")
	if err != nil {
		t.Fatalf("ComputeSessionKey error: %v", err)
	}
	k2, err := ComputeSessionKey("local", convB, "default")
	if err != nil {
		t.Fatalf("ComputeSessionKey error: %v", err)
	}
	if k1 != k2 {
		t.Fatalf("expected same key when only thread_id differs, got %s != %s", k1, k2)
	}
}

func TestComputeSessionKey_DMRequiresParticipant(t *testing.T) {
	_, err := ComputeSessionKey("local", model.Conversation{ConversationID: "conv_1", ChannelType: "dm"}, "default")
	if err == nil {
		t.Fatalf("expected error for missing participant_id")
	}
}
