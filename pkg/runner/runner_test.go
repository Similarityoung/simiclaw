package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRememberInDMDoesNotWriteCurated(t *testing.T) {
	workspace := t.TempDir()
	r := NewProcessRunner(workspace, nil)

	out, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_remember_dm",
		SessionKey:      "sk_dm",
		ActiveSessionID: "s_dm",
		Conversation: model.Conversation{
			ConversationID: "conv_dm",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		Payload: model.EventPayload{
			Type: "message",
			Text: "记住我喜欢 Go",
		},
	}, 4)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if hasCuratedWrite(out.Trace.Actions) {
		t.Fatalf("dm remember should not write curated memory, actions=%+v", out.Trace.Actions)
	}
	if _, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("dm remember should not create MEMORY.md, stat err=%v", err)
	}
}

func TestCompactionInDMDoesNotWriteCurated(t *testing.T) {
	workspace := t.TempDir()
	r := NewProcessRunner(workspace, nil)

	out, err := r.Run(context.Background(), model.InternalEvent{
		EventID:         "evt_compaction_dm",
		SessionKey:      "sk_dm",
		ActiveSessionID: "s_dm",
		Conversation: model.Conversation{
			ConversationID: "conv_dm",
			ChannelType:    "dm",
			ParticipantID:  "u1",
		},
		Payload: model.EventPayload{
			Type: "compaction",
		},
	}, 4)
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	if hasCuratedWrite(out.Trace.Actions) {
		t.Fatalf("dm compaction should not write curated memory, actions=%+v", out.Trace.Actions)
	}
	if _, err := os.Stat(filepath.Join(workspace, "MEMORY.md")); !os.IsNotExist(err) {
		t.Fatalf("dm compaction should not create MEMORY.md, stat err=%v", err)
	}
}

func hasCuratedWrite(actions []model.Action) bool {
	for _, act := range actions {
		if act.Type != "WriteMemory" {
			continue
		}
		target, _ := act.Payload["target"].(string)
		if target == "curated" {
			return true
		}
	}
	return false
}
