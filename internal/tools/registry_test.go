package tools

import (
	"context"
	"slices"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func TestRegistryInvokePassesCapabilityContextAndReturnsHandlerOutput(t *testing.T) {
	reg := NewRegistry()
	reg.Register("echo", Schema{
		Name:        "echo",
		Description: "echo",
		Parameters:  ParameterSchema{Type: "object"},
	}, func(_ context.Context, toolCtx Context, args map[string]any) Result {
		return Result{Output: map[string]any{
			"workspace": toolCtx.Workspace,
			"channel":   toolCtx.Conversation.ChannelType,
			"query":     args["query"],
		}}
	})

	res := reg.Invoke(context.Background(), kernel.ToolContext{
		Workspace:    "/tmp/ws",
		Conversation: model.Conversation{ChannelType: "dm"},
	}, "echo", map[string]any{"query": "hello"})
	if res.Error != nil {
		t.Fatalf("unexpected error: %+v", res.Error)
	}
	if res.Output["workspace"] != "/tmp/ws" || res.Output["channel"] != "dm" || res.Output["query"] != "hello" {
		t.Fatalf("unexpected output: %+v", res.Output)
	}
}

func TestRegistryInvokeWrapsPanicAsInternalError(t *testing.T) {
	reg := NewRegistry()
	reg.Register("panic_tool", Schema{Name: "panic_tool"}, func(context.Context, Context, map[string]any) Result {
		panic("boom")
	})

	res := reg.Invoke(context.Background(), kernel.ToolContext{}, "panic_tool", nil)
	if res.Error == nil || res.Error.Code != model.ErrorCodeInternal || res.Error.Message != "tool panic: boom" {
		t.Fatalf("unexpected panic result: %+v", res.Error)
	}
}

func TestRegistryInvokeTurnsContextCancellationIntoCapabilityErrorSurface(t *testing.T) {
	reg := NewRegistry()
	called := false
	reg.Register("slow_tool", Schema{Name: "slow_tool"}, func(context.Context, Context, map[string]any) Result {
		called = true
		return Result{}
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	res := reg.Invoke(ctx, kernel.ToolContext{}, "slow_tool", nil)
	if res.Error == nil {
		t.Fatal("expected cancellation error")
	}
	if res.Error.Code != model.ErrorCodeCanceled {
		t.Fatalf("error code = %q want %q", res.Error.Code, model.ErrorCodeCanceled)
	}
	if got := res.Error.Details["capability_error_kind"]; got != string(kernel.CapabilityErrorCanceled) {
		t.Fatalf("capability_error_kind = %v want %q", got, kernel.CapabilityErrorCanceled)
	}
	if called {
		t.Fatal("expected canceled context to short-circuit before handler execution")
	}
}

func TestRegistryInvokeDoesNotOverwriteSuccessfulResultWhenContextCancelsDuringHandler(t *testing.T) {
	reg := NewRegistry()
	ctx, cancel := context.WithCancel(context.Background())
	reg.Register("write_like_tool", Schema{Name: "write_like_tool"}, func(context.Context, Context, map[string]any) Result {
		cancel()
		return Result{Output: map[string]any{"status": "ok"}}
	})

	res := reg.Invoke(ctx, kernel.ToolContext{}, "write_like_tool", nil)
	if res.Error != nil {
		t.Fatalf("expected success result to be preserved, got %+v", res.Error)
	}
	if res.Output["status"] != "ok" {
		t.Fatalf("unexpected output: %+v", res.Output)
	}
}

func TestRegistryToolDefinitionsAreSortedByName(t *testing.T) {
	reg := NewRegistry()
	reg.Register("zeta", Schema{Name: "zeta"}, func(context.Context, Context, map[string]any) Result { return Result{} })
	reg.Register("alpha", Schema{Name: "alpha"}, func(context.Context, Context, map[string]any) Result { return Result{} })
	reg.Register("beta", Schema{Name: "beta"}, func(context.Context, Context, map[string]any) Result { return Result{} })

	defs := reg.ToolDefinitions()
	names := make([]string, 0, len(defs))
	for _, def := range defs {
		names = append(names, def.Name)
	}
	if !slices.Equal(names, []string{"alpha", "beta", "zeta"}) {
		t.Fatalf("unexpected tool definition order: %+v", names)
	}
}
