package tx

import (
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/gateway"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestPersistEventDuplicateReturnsStoredSessionKey(t *testing.T) {
	repo := newTestRuntimeRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	req := gateway.PersistRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
		IdempotencyKey: "cli:duplicate:1",
	}

	first, err := repo.PersistEvent(ctx, "local", "local:dm:u1", req, "sha256:duplicate", now)
	if err != nil {
		t.Fatalf("first PersistEvent: %v", err)
	}
	duplicate, err := repo.PersistEvent(ctx, "local", "local:dm:u1:changed", req, "sha256:duplicate", now.Add(time.Second))
	if err != nil {
		t.Fatalf("duplicate PersistEvent: %v", err)
	}

	if !duplicate.Duplicate {
		t.Fatalf("expected duplicate result, got %+v", duplicate)
	}
	if duplicate.SessionKey != first.SessionKey {
		t.Fatalf("expected stored session key %q, got %+v", first.SessionKey, duplicate)
	}
	if duplicate.SessionID != first.SessionID {
		t.Fatalf("expected stored session id %q, got %+v", first.SessionID, duplicate)
	}
}

func TestListRunnableCarriesSessionLane(t *testing.T) {
	repo := newTestRuntimeRepository(t)
	ctx := context.Background()
	now := time.Now().UTC()
	req := gateway.PersistRequest{
		Source:         "cli",
		Conversation:   model.Conversation{ConversationID: "conv", ChannelType: "dm", ParticipantID: "u1"},
		Payload:        model.EventPayload{Type: "message", Text: "hello"},
		IdempotencyKey: "cli:runnable:1",
	}

	result, err := repo.PersistEvent(ctx, "local", "local:dm:u1", req, "sha256:runnable", now)
	if err != nil {
		t.Fatalf("PersistEvent: %v", err)
	}
	items, err := repo.ListRunnable(ctx, 10)
	if err != nil {
		t.Fatalf("ListRunnable: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one runnable item, got %+v", items)
	}
	if items[0].EventID != result.EventID {
		t.Fatalf("expected runnable event %q, got %+v", result.EventID, items[0])
	}
	if items[0].SessionKey != result.SessionKey {
		t.Fatalf("expected session key %q, got %+v", result.SessionKey, items[0])
	}
	if items[0].LaneKey != "session:"+result.SessionKey {
		t.Fatalf("expected session lane, got %+v", items[0])
	}
}

func newTestRuntimeRepository(t *testing.T) *RuntimeRepository {
	t.Helper()
	workspace := t.TempDir()
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	db, err := store.Open(workspace, store.DefaultBusyTimeout())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return NewRuntimeRepository(db)
}
