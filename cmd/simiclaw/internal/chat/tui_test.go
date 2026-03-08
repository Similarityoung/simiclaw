package chat

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	sharedclient "github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/client"
	"github.com/similarityyoung/simiclaw/cmd/simiclaw/internal/common"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestModelInitWithConversationLoadsExistingHistory(t *testing.T) {
	cli := newTUITestClient(t, map[string][]model.SessionRecord{
		"conv-existing": {
			{
				SessionKey:     "sk_group",
				ConversationID: "conv-existing",
				ChannelType:    "group",
				ParticipantID:  "other_user",
				LastActivityAt: time.Now().UTC(),
				CreatedAt:      time.Now().UTC(),
				UpdatedAt:      time.Now().UTC(),
			},
			{
				SessionKey:      "sk_dm",
				ActiveSessionID: "ses_dm",
				ConversationID:  "conv-existing",
				ChannelType:     "dm",
				ParticipantID:   fixedParticipantID,
				LastActivityAt:  time.Now().UTC(),
				CreatedAt:       time.Now().UTC(),
				UpdatedAt:       time.Now().UTC(),
			},
		},
	}, map[string][]model.MessageRecord{
		"sk_dm": {
			{Role: "user", Content: "hello", Visible: true},
			{Role: "assistant", Content: "world", Visible: true},
		},
	})

	m := newModel(common.IOStreams{}, cli, Options{Conversation: "conv-existing", HistoryLimit: 20})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command to load conversation history")
	}

	updated, _ := m.Update(cmd())
	got := updated.(*modelState)
	if got.mode != modeChat {
		t.Fatalf("mode = %v want %v", got.mode, modeChat)
	}
	if got.sessionKey != "sk_dm" {
		t.Fatalf("sessionKey = %q want %q", got.sessionKey, "sk_dm")
	}
	if len(got.messages) != 2 {
		t.Fatalf("messages len = %d want 2", len(got.messages))
	}
	if got.messages[0].Content != "hello" || got.messages[1].Content != "world" {
		t.Fatalf("unexpected messages = %+v", got.messages)
	}
}

func TestHandleNamingKeyRejectsExistingConversationID(t *testing.T) {
	cli := newTUITestClient(t, map[string][]model.SessionRecord{
		"dup-conversation": {
			{
				SessionKey:      "sk_dup",
				ActiveSessionID: "ses_dup",
				ConversationID:  "dup-conversation",
				ChannelType:     "dm",
				ParticipantID:   fixedParticipantID,
				LastActivityAt:  time.Now().UTC(),
				CreatedAt:       time.Now().UTC(),
				UpdatedAt:       time.Now().UTC(),
			},
		},
	}, nil)

	m := newModel(common.IOStreams{}, cli, Options{})
	m.mode = modeNaming
	m.nameInput.SetValue("dup-conversation")

	updated, cmd := m.handleNamingKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected enter to trigger availability check")
	}
	updated, _ = updated.Update(cmd())
	got := updated.(*modelState)
	if got.mode != modeNaming {
		t.Fatalf("mode = %v want %v", got.mode, modeNaming)
	}
	if got.loading {
		t.Fatal("expected loading to be cleared after check")
	}
	if got.nameInput.Value() != "dup-conversation" {
		t.Fatalf("name input = %q want %q", got.nameInput.Value(), "dup-conversation")
	}
	if !strings.Contains(got.status, "已存在") {
		t.Fatalf("status = %q, want duplicate warning", got.status)
	}
}

func TestModelInitWithNewConversationRejectsExistingConversationID(t *testing.T) {
	cli := newTUITestClient(t, map[string][]model.SessionRecord{
		"dup-conversation": {
			{
				SessionKey:      "sk_dup",
				ActiveSessionID: "ses_dup",
				ConversationID:  "dup-conversation",
				ChannelType:     "dm",
				ParticipantID:   fixedParticipantID,
				LastActivityAt:  time.Now().UTC(),
				CreatedAt:       time.Now().UTC(),
				UpdatedAt:       time.Now().UTC(),
			},
		},
	}, nil)

	m := newModel(common.IOStreams{}, cli, Options{NewSession: true, Conversation: "dup-conversation"})
	cmd := m.Init()
	if cmd == nil {
		t.Fatal("expected init command to validate new conversation")
	}

	updated, _ := m.Update(cmd())
	got := updated.(*modelState)
	if got.mode != modeNaming {
		t.Fatalf("mode = %v want %v", got.mode, modeNaming)
	}
	if got.nameInput.Value() != "dup-conversation" {
		t.Fatalf("name input = %q want %q", got.nameInput.Value(), "dup-conversation")
	}
	if !strings.Contains(got.status, "已存在") {
		t.Fatalf("status = %q, want duplicate warning", got.status)
	}
}

func newTUITestClient(t *testing.T, sessionsByConversation map[string][]model.SessionRecord, historyBySession map[string][]model.MessageRecord) *sharedclient.Client {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sessions", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		conversation := r.URL.Query().Get("conversation_id")
		_ = json.NewEncoder(w).Encode(sharedclient.SessionPage{Items: sessionsByConversation[conversation]})
	})
	mux.HandleFunc("/v1/sessions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || !strings.HasSuffix(r.URL.Path, "/history") {
			http.NotFound(w, r)
			return
		}
		sessionKey := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/sessions/"), "/history")
		_ = json.NewEncoder(w).Encode(sharedclient.MessagePage{Items: historyBySession[sessionKey]})
	})

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return sharedclient.New(server.URL, "", time.Second)
}
