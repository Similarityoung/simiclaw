package httpapi

import (
	"testing"
	"time"

	querymodel "github.com/similarityyoung/simiclaw/internal/query/model"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestToAPIRunTraceMapsRichTraceFields(t *testing.T) {
	now := time.Now().UTC()
	trace := querymodel.RunTrace{
		RunID:      "run_1",
		EventID:    "evt_1",
		SessionKey: "local:dm:u1",
		SessionID:  "ses_1",
		RunMode:    model.RunModeNormal,
		Status:     model.RunStatusCompleted,
		ContextManifest: &querymodel.ContextManifest{
			HistoryRange: querymodel.HistoryRange{
				Mode:      "tail",
				TailLimit: 12,
			},
		},
		RAGHits: []querymodel.RAGHit{{
			Path:    "memory/public/MEMORY.md",
			Scope:   "public",
			Lines:   []int{3, 4},
			Score:   0.75,
			Preview: "remember this",
		}},
		ToolExecutions: []querymodel.ToolExecution{{
			ToolCallID:  "call_1",
			Name:        "memory_search",
			Args:        map[string]any{"query": "alpha"},
			ArgsSummary: map[string]any{"query": "alpha"},
			Result:      map[string]any{"hits": 1},
			Error:       &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: "boom"},
		}},
		Actions: []querymodel.Action{{
			ActionID:             "act_1",
			ActionIndex:          1,
			ActionIdempotencyKey: "idem_1",
			Type:                 "tool",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"tool": "memory_search"},
		}},
		StartedAt:   now,
		FinishedAt:  now.Add(time.Second),
		Provider:    "fake",
		Model:       "default",
		ToolCalls:   []model.ToolCall{{ToolCallID: "call_1", Name: "memory_search"}},
		Diagnostics: map[string]string{"source": "test"},
	}

	got := toAPIRunTrace(trace)

	if got.ContextManifest == nil || got.ContextManifest.HistoryRange.Mode != "tail" || got.ContextManifest.HistoryRange.TailLimit != 12 {
		t.Fatalf("unexpected context manifest: %+v", got.ContextManifest)
	}
	if len(got.RAGHits) != 1 || got.RAGHits[0].Path != "memory/public/MEMORY.md" || got.RAGHits[0].Preview != "remember this" {
		t.Fatalf("unexpected rag hits: %+v", got.RAGHits)
	}
	if len(got.ToolExecutions) != 1 || got.ToolExecutions[0].ToolCallID != "call_1" || got.ToolExecutions[0].Error == nil || got.ToolExecutions[0].Error.Message != "boom" {
		t.Fatalf("unexpected tool executions: %+v", got.ToolExecutions)
	}
	if len(got.Actions) != 1 || got.Actions[0].ActionID != "act_1" || got.Actions[0].Type != "tool" {
		t.Fatalf("unexpected actions: %+v", got.Actions)
	}
}
