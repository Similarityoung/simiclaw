package runner

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Runner interface {
	Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error)
}

type RunOutput struct {
	RunID          string
	RunMode        model.RunMode
	Entries        []model.SessionEntry
	Trace          model.RunTrace
	OutboundBody   string
	SuppressOutput bool
}

var idSeq atomic.Uint64

func nextID(prefix string, now time.Time) string {
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}

type ProcessRunner struct{}

func NewProcessRunner() *ProcessRunner {
	return &ProcessRunner{}
}

func (r *ProcessRunner) Run(_ context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error) {
	now := time.Now().UTC()
	runID := nextID("run", now)

	mode := model.RunModeNormal
	if event.Payload.Type == "memory_flush" || event.Payload.Type == "compaction" || event.Payload.Type == "cron_fire" {
		mode = model.RunModeNoReply
	}

	entries := []model.SessionEntry{{
		Type:    "user",
		EntryID: nextID("e_user", now),
		RunID:   runID,
		Content: event.Payload.Text,
	}}
	actions := []model.Action{}
	outbound := ""

	if mode == model.RunModeNoReply {
		actions = append(actions, model.Action{
			ActionID:             nextID("act", now),
			ActionIndex:          0,
			ActionIdempotencyKey: fmt.Sprintf("%s:0", runID),
			Type:                 "WriteMemory",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"reason": event.Payload.Type},
		})
	} else if strings.Contains(event.Payload.Text, "/tool") && maxToolRounds > 0 {
		toolCallID := nextID("tc", now)
		entries = append(entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant_tool", now),
			RunID:   runID,
			ToolCalls: []model.ToolCall{{
				ToolCallID: toolCallID,
				Name:       "mock_tool",
				Args:       map[string]any{"query": event.Payload.Text},
			}},
		})
		entries = append(entries, model.SessionEntry{
			Type:       "tool_result",
			EntryID:    nextID("e_tool_result", now),
			RunID:      runID,
			ToolCallID: toolCallID,
			OK:         true,
			Result:     "tool_result_ok",
		})
		final := "工具执行完成"
		entries = append(entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant_final", now),
			RunID:   runID,
			Content: final,
		})
		outbound = final
		actions = append(actions, model.Action{
			ActionID:             nextID("act", now),
			ActionIndex:          0,
			ActionIdempotencyKey: fmt.Sprintf("%s:0", runID),
			Type:                 "Invoke",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"tool": "mock_tool"},
		})
	} else {
		reply := "已收到: " + event.Payload.Text
		entries = append(entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		outbound = reply
		actions = append(actions, model.Action{
			ActionID:             nextID("act", now),
			ActionIndex:          0,
			ActionIdempotencyKey: fmt.Sprintf("%s:0", runID),
			Type:                 "SendMessage",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"text": reply},
		})
	}

	trace := model.RunTrace{
		RunID:      runID,
		EventID:    event.EventID,
		SessionKey: event.SessionKey,
		SessionID:  event.ActiveSessionID,
		RunMode:    mode,
		Actions:    actions,
		StartedAt:  now,
		FinishedAt: time.Now().UTC(),
	}

	return RunOutput{
		RunID:          runID,
		RunMode:        mode,
		Entries:        entries,
		Trace:          trace,
		OutboundBody:   outbound,
		SuppressOutput: mode == model.RunModeNoReply,
	}, nil
}
