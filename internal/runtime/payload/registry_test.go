package payload

import (
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRegistryResolveBuiltinsAndFallback(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)

	testCases := []struct {
		name        string
		payloadType string
		check       func(t *testing.T, plan Plan)
	}{
		{
			name:        "message uses interactive plan",
			payloadType: "message",
			check: func(t *testing.T, plan Plan) {
				t.Helper()
				if plan.RunMode != model.RunModeNormal || plan.Kind != ExecutionKindInteractive {
					t.Fatalf("unexpected message plan: %+v", plan)
				}
				if plan.SuppressOutput || plan.SuppressStream {
					t.Fatalf("message plan should stay visible: %+v", plan)
				}
			},
		},
		{
			name:        "memory flush writes daily memory without reply",
			payloadType: "memory_flush",
			check: func(t *testing.T, plan Plan) {
				t.Helper()
				if plan.RunMode != model.RunModeNoReply || plan.Kind != ExecutionKindMemoryWrite {
					t.Fatalf("unexpected memory_flush plan: %+v", plan)
				}
				if !plan.SuppressOutput || !plan.SuppressStream || plan.MemoryWriteTarget != MemoryWriteTargetDaily {
					t.Fatalf("unexpected memory_flush behavior: %+v", plan)
				}
				if got := plan.MessageMeta["payload_type"]; got != "memory_flush" {
					t.Fatalf("unexpected memory_flush meta: %#v", plan.MessageMeta)
				}
			},
		},
		{
			name:        "compaction writes curated memory without reply",
			payloadType: "compaction",
			check: func(t *testing.T, plan Plan) {
				t.Helper()
				if plan.RunMode != model.RunModeNoReply || plan.Kind != ExecutionKindMemoryWrite {
					t.Fatalf("unexpected compaction plan: %+v", plan)
				}
				if !plan.SuppressOutput || !plan.SuppressStream || plan.MemoryWriteTarget != MemoryWriteTargetCurated {
					t.Fatalf("unexpected compaction behavior: %+v", plan)
				}
				if got := plan.MessageMeta["payload_type"]; got != "compaction" {
					t.Fatalf("unexpected compaction meta: %#v", plan.MessageMeta)
				}
			},
		},
		{
			name:        "cron fire stays suppressed and allowlisted",
			payloadType: "cron_fire",
			check: func(t *testing.T, plan Plan) {
				t.Helper()
				if plan.RunMode != model.RunModeNoReply || plan.Kind != ExecutionKindSuppressedLLM {
					t.Fatalf("unexpected cron_fire plan: %+v", plan)
				}
				if !plan.SuppressOutput || !plan.SuppressStream {
					t.Fatalf("cron_fire should suppress output and stream: %+v", plan)
				}
				if plan.UserVisible || plan.ToolVisible || plan.FinalAssistantVisible {
					t.Fatalf("cron_fire messages should stay hidden: %+v", plan)
				}
				for _, toolName := range []string{"memory_search", "memory_get", "context_get"} {
					if _, ok := plan.AllowedTools[toolName]; !ok {
						t.Fatalf("missing allowed tool %q in %+v", toolName, plan.AllowedTools)
					}
				}
			},
		},
		{
			name:        "unknown payload falls back to interactive plan",
			payloadType: "unknown_payload",
			check: func(t *testing.T, plan Plan) {
				t.Helper()
				if plan.RunMode != model.RunModeNormal || plan.Kind != ExecutionKindInteractive {
					t.Fatalf("unexpected fallback plan: %+v", plan)
				}
				if plan.SuppressOutput || plan.SuppressStream {
					t.Fatalf("fallback plan should stay interactive: %+v", plan)
				}
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t, registry.Resolve(tc.payloadType))
		})
	}
}

func TestRegistryResolveClonesMutableFields(t *testing.T) {
	registry := NewRegistry()
	RegisterBuiltins(registry)

	first := registry.Resolve("cron_fire")
	delete(first.AllowedTools, "memory_search")
	first.MessageMeta["payload_type"] = "mutated"

	second := registry.Resolve("cron_fire")
	if _, ok := second.AllowedTools["memory_search"]; !ok {
		t.Fatalf("expected allowed tools to be isolated across resolves, got %+v", second.AllowedTools)
	}
	if got := second.MessageMeta["payload_type"]; got != "cron_fire" {
		t.Fatalf("expected message meta clone, got %#v", second.MessageMeta)
	}
}
