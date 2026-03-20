package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func TestProviderRunnerLogsProviderAndToolMilestones(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register("echo", tools.Schema{Name: "echo"}, func(_ context.Context, _ tools.Context, args map[string]any) tools.Result {
		return tools.Result{Output: map[string]any{"echo": args["query"]}}
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "done: {{last_user_message}}",
		FakeToolName:         "echo",
		FakeToolArgsJSON:     `{"query":"hello"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-1",
	}
	r := newTestRunner(t, cfg, registry)

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		if _, err := r.Run(WithRunID(context.Background(), "run_log"), testEvent("log it"), 2, nil); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[runner] payload plan selected",
		"[runner] provider started",
		"[runner] provider completed",
		"[runner.tool] tool started",
		"[runner.tool] tool finished",
		"[runner] run completed",
	)
	for _, part := range []string{
		`"event_id": "evt_test"`,
		`"run_id": "run_log"`,
		`"provider": "fake"`,
		`"model": "default"`,
		`"tool_call_id": "fake-tool-call-1"`,
		`"tool_name": "echo"`,
		`"tool_rounds": 1`,
	} {
		if !strings.Contains(out, part) {
			t.Fatalf("missing %q in %q", part, out)
		}
	}
}

func TestProviderRunnerLogsToolPolicyRejection(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register("echo", tools.Schema{Name: "echo"}, func(_ context.Context, _ tools.Context, args map[string]any) tools.Result {
		return tools.Result{Output: map[string]any{"echo": args["query"]}}
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "done",
		FakeToolName:         "echo",
		FakeToolArgsJSON:     `{"query":"cron task"}`,
		FakeFinishReason:     "stop",
		FakeRawFinishReason:  "stop",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-2",
	}
	r := newTestRunner(t, cfg, registry)
	event := testEvent("cron task")
	event.Payload.Type = "cron_fire"

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		if _, err := r.Run(WithRunID(context.Background(), "run_cron"), event, 2, nil); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		_ = logging.Sync()
	})

	if !strings.Contains(out, "[runner.tool] tool denied") {
		t.Fatalf("expected tool denied log, got %q", out)
	}
	if !strings.Contains(out, `"error_code": "FORBIDDEN"`) {
		t.Fatalf("expected forbidden error code, got %q", out)
	}
}

func TestProviderRunnerLogsToolRoundsExhausted(t *testing.T) {
	registry := tools.NewRegistry()
	registry.Register("echo", tools.Schema{Name: "echo"}, func(_ context.Context, _ tools.Context, args map[string]any) tools.Result {
		return tools.Result{Output: map[string]any{"echo": args["query"]}}
	})
	cfg := config.Default().LLM
	cfg.DefaultModel = "fake/default"
	cfg.Providers["fake"] = config.LLMProviderConfig{
		Type:                 "fake",
		FakeResponseText:     "",
		FakeToolName:         "echo",
		FakeToolArgsJSON:     `{"query":"hello"}`,
		FakeFinishReason:     "tool_calls",
		FakeRawFinishReason:  "tool_calls",
		FakePromptTokens:     8,
		FakeCompletionTokens: 8,
		FakeRequestID:        "fake-request-3",
	}
	r := newTestRunner(t, cfg, registry)

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		if _, err := r.Run(WithRunID(context.Background(), "run_budget"), testEvent("budget"), 0, nil); err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
		_ = logging.Sync()
	})

	if !strings.Contains(out, "[runner] tool rounds exhausted") {
		t.Fatalf("expected tool rounds exhausted log, got %q", out)
	}
	if !strings.Contains(out, `"max_tool_rounds": 0`) {
		t.Fatalf("expected max_tool_rounds field, got %q", out)
	}
}

func TestWithRunIDNilContextReturnsNil(t *testing.T) {
	if got := WithRunID(nil, "run_nil"); got != nil {
		t.Fatalf("WithRunID(nil, run_nil)=%v want nil", got)
	}
}
