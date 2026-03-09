package chat

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/similarityyoung/simiclaw/internal/ui/messages"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRunREPLRecoversFromStreamInterruptionWithoutDuplicatePrint(t *testing.T) {
	client := &stubChatClient{
		sendStream: func(ctx context.Context, req model.IngestRequest, handler StreamEventHandler) (model.EventRecord, error) {
			if err := handler.HandleStreamEvent(model.ChatStreamEvent{
				Type:                  model.ChatStreamEventAccepted,
				EventID:               "evt_1",
				Sequence:              1,
				StreamProtocolVersion: model.ChatStreamProtocolVersion,
			}); err != nil {
				return model.EventRecord{}, err
			}
			if err := handler.HandleStreamEvent(model.ChatStreamEvent{
				Type:     model.ChatStreamEventTextDelta,
				EventID:  "evt_1",
				Sequence: 2,
				Delta:    "hel",
			}); err != nil {
				return model.EventRecord{}, err
			}
			return model.EventRecord{}, &StreamRecoverableError{
				EventID: "evt_1",
				Err:     errors.New("unexpected EOF"),
			}
		},
		pollEvent: func(context.Context, string) (model.EventRecord, error) {
			return model.EventRecord{
				EventID:        "evt_1",
				Status:         model.EventStatusProcessed,
				AssistantReply: "hello world",
			}, nil
		},
	}

	var out bytes.Buffer
	err := runREPL(context.Background(), strings.NewReader("hello\n/quit\n"), &out, client, "conv", true, time.Now)
	if err != nil {
		t.Fatalf("runREPL returned error: %v", err)
	}
	want := messages.Chat.REPLPrompt + messages.Chat.StreamBotPrompt + "hel" + messages.Chat.StreamBotRewrite("hello world") + messages.Chat.REPLPrompt
	if out.String() != want {
		t.Fatalf("unexpected output:\nwant: %q\ngot:  %q", want, out.String())
	}
}

func TestRunREPLEmptyDoneStillCompletesTurn(t *testing.T) {
	client := &stubChatClient{
		sendStream: func(ctx context.Context, req model.IngestRequest, handler StreamEventHandler) (model.EventRecord, error) {
			if err := handler.HandleStreamEvent(model.ChatStreamEvent{
				Type:                  model.ChatStreamEventAccepted,
				EventID:               "evt_2",
				Sequence:              1,
				StreamProtocolVersion: model.ChatStreamProtocolVersion,
			}); err != nil {
				return model.EventRecord{}, err
			}
			return model.EventRecord{
				EventID:        "evt_2",
				Status:         model.EventStatusProcessed,
				AssistantReply: "",
			}, nil
		},
	}

	var out bytes.Buffer
	err := runREPL(context.Background(), strings.NewReader("hello\n/quit\n"), &out, client, "conv", true, time.Now)
	if err != nil {
		t.Fatalf("runREPL returned error: %v", err)
	}
	want := messages.Chat.REPLPrompt + messages.Chat.StreamBotPrompt + messages.Chat.StreamNoReply + "\n" + messages.Chat.REPLPrompt
	if out.String() != want {
		t.Fatalf("unexpected output:\nwant: %q\ngot:  %q", want, out.String())
	}
}

type stubChatClient struct {
	sendAndWait func(ctx context.Context, req model.IngestRequest) (model.EventRecord, error)
	sendStream  func(ctx context.Context, req model.IngestRequest, handler StreamEventHandler) (model.EventRecord, error)
	pollEvent   func(ctx context.Context, eventID string) (model.EventRecord, error)
}

func (s *stubChatClient) SendAndWait(ctx context.Context, req model.IngestRequest) (model.EventRecord, error) {
	if s.sendAndWait == nil {
		return model.EventRecord{}, errors.New("unexpected SendAndWait")
	}
	return s.sendAndWait(ctx, req)
}

func (s *stubChatClient) SendStream(ctx context.Context, req model.IngestRequest, handler StreamEventHandler) (model.EventRecord, error) {
	if s.sendStream == nil {
		return model.EventRecord{}, errors.New("unexpected SendStream")
	}
	return s.sendStream(ctx, req, handler)
}

func (s *stubChatClient) PollEvent(ctx context.Context, eventID string) (model.EventRecord, error) {
	if s.pollEvent == nil {
		return model.EventRecord{}, errors.New("unexpected PollEvent")
	}
	return s.pollEvent(ctx, eventID)
}
