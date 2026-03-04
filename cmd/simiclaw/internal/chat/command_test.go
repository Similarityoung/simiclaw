package chat

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.BaseURL != defaultBaseURL {
		t.Fatalf("unexpected base url: %s", cfg.BaseURL)
	}
	if cfg.Conversation != defaultConversation {
		t.Fatalf("unexpected conversation: %s", cfg.Conversation)
	}
	if cfg.APIKey != "" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
}

func TestParseConfigOverrides(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--base-url", "http://127.0.0.1:19090/",
		"--conversation", "demo_room",
		"--api-key", "secret",
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:19090" {
		t.Fatalf("unexpected base url: %s", cfg.BaseURL)
	}
	if cfg.Conversation != "demo_room" {
		t.Fatalf("unexpected conversation: %s", cfg.Conversation)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
}

func TestRunREPLSendAndQuit(t *testing.T) {
	in := bytes.NewBufferString("hello\n/quit\n")
	out := &bytes.Buffer{}
	client := &fakeClient{
		records: []model.EventRecord{
			{
				Status:         model.EventStatusCommitted,
				DeliveryStatus: model.DeliveryStatusSent,
				AssistantReply: "已收到: hello",
			},
		},
	}
	now := func() time.Time { return time.UnixMilli(12345) }

	if err := runREPL(context.Background(), in, out, client, "demo", now); err != nil {
		t.Fatalf("runREPL: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: %d", len(client.requests))
	}
	if got := client.requests[0].IdempotencyKey; got != "cli:demo:12345" {
		t.Fatalf("unexpected idempotency key: %s", got)
	}
	if got := out.String(); !bytes.Contains([]byte(got), []byte("bot> 已收到: hello")) {
		t.Fatalf("unexpected output: %q", got)
	}
}

type fakeClient struct {
	records  []model.EventRecord
	errors   []error
	requests []model.IngestRequest
}

func (f *fakeClient) SendAndWait(_ context.Context, req model.IngestRequest) (model.EventRecord, error) {
	idx := len(f.requests)
	f.requests = append(f.requests, req)
	if idx < len(f.errors) && f.errors[idx] != nil {
		return model.EventRecord{}, f.errors[idx]
	}
	if idx < len(f.records) {
		return f.records[idx], nil
	}
	return model.EventRecord{}, nil
}
