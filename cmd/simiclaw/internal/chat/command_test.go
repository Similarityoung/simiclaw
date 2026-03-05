package chat

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
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
	if cfg.Participant != defaultParticipant {
		t.Fatalf("unexpected participant: %s", cfg.Participant)
	}
	if cfg.APIKey != "" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
}

func TestParseConfigOverrides(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--base-url", "http://127.0.0.1:19090/",
		"--conversation", "demo_room",
		"--participant", "u_demo",
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
	if cfg.Participant != "u_demo" {
		t.Fatalf("unexpected participant: %s", cfg.Participant)
	}
	if cfg.APIKey != "secret" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
}

func TestParseConfigFromGatewayConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	data := []byte(`{"workspace":".","listen_addr":":19091","api_key":"cfg_secret"}`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := parseConfig([]string{"--config", cfgPath})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:19091" {
		t.Fatalf("unexpected base url: %s", cfg.BaseURL)
	}
	if cfg.APIKey != "cfg_secret" {
		t.Fatalf("unexpected api key: %s", cfg.APIKey)
	}
	if cfg.Participant != defaultParticipant {
		t.Fatalf("unexpected participant: %s", cfg.Participant)
	}
}

func TestParseConfigCLIOverridesGatewayConfig(t *testing.T) {
	tmp := t.TempDir()
	cfgPath := filepath.Join(tmp, "config.json")
	data := []byte(`{"workspace":".","listen_addr":":19091","api_key":"cfg_secret"}`)
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := parseConfig([]string{
		"--config", cfgPath,
		"--base-url", "http://127.0.0.1:19092",
		"--api-key", "flag_secret",
	})
	if err != nil {
		t.Fatalf("parseConfig: %v", err)
	}
	if cfg.BaseURL != "http://127.0.0.1:19092" {
		t.Fatalf("unexpected base url: %s", cfg.BaseURL)
	}
	if cfg.APIKey != "flag_secret" {
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

	if err := runREPL(context.Background(), in, out, client, "demo", "user_demo", now); err != nil {
		t.Fatalf("runREPL: %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("unexpected request count: %d", len(client.requests))
	}
	if got := client.requests[0].IdempotencyKey; got != "cli:demo:12345" {
		t.Fatalf("unexpected idempotency key: %s", got)
	}
	if got := client.requests[0].Conversation.ParticipantID; got != "user_demo" {
		t.Fatalf("unexpected participant: %s", got)
	}
	if got := out.String(); !bytes.Contains([]byte(got), []byte("bot> 已收到: hello")) {
		t.Fatalf("unexpected output: %q", got)
	}
}

func TestRunREPLCancelWhileWaitingInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reader, writer := io.Pipe()
	defer writer.Close()

	out := &bytes.Buffer{}
	client := &fakeClient{}
	done := make(chan error, 1)
	go func() {
		done <- runREPL(ctx, reader, out, client, "demo", "user_demo", time.Now)
	}()

	deadline := time.Now().Add(300 * time.Millisecond)
	for {
		if bytes.Contains(out.Bytes(), []byte("you> ")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("prompt not printed, output=%q", out.String())
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runREPL: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("runREPL did not exit after context cancellation")
	}

	if len(client.requests) != 0 {
		t.Fatalf("unexpected request count: %d", len(client.requests))
	}
}

func TestFormatErrorWithoutCode(t *testing.T) {
	err := formatError(&APIError{StatusCode: 500, Message: "gateway unavailable"})
	if err != "gateway unavailable" {
		t.Fatalf("unexpected formatted error: %q", err)
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
