package runner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/similarityyoung/simiclaw/internal/tools"
	"github.com/similarityyoung/simiclaw/pkg/api"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

type executedToolStep struct {
	message   OutputMessage
	chat      providerToolMessage
	execution api.ToolExecution
	action    api.Action
}

type providerToolMessage struct {
	role       string
	content    string
	toolCallID string
}

type llmToolExecutor struct {
	workspace string
	registry  *tools.Registry
}

func (e llmToolExecutor) Execute(ctx context.Context, event model.InternalEvent, call model.ToolCall, opts llmRunOptions, counts map[string]int, sink StreamSink, actionIndex int, now time.Time) executedToolStep {
	displayArgs, argsTruncated := sanitizeDisplayMap(call.Args)
	sink.OnToolStart(call.ToolCallID, call.Name, displayArgs, argsTruncated)

	res := e.call(ctx, event, call, opts.allowedTools, counts)
	exec := api.ToolExecution{
		ToolCallID: call.ToolCallID,
		Name:       call.Name,
		Args:       call.Args,
		Result:     res.Output,
		Error:      res.Error,
	}
	action := api.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          actionIndex,
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", event.EventID, actionIndex),
		Type:                 "InvokeTool",
		Risk:                 toolRisk(call.Name),
		Payload:              map[string]any{"tool_name": call.Name},
	}

	payload := map[string]any{}
	if res.Output != nil {
		payload = res.Output
	}
	displayResult, resultTruncated := sanitizeDisplayMap(payload)
	sink.OnToolResult(call.ToolCallID, call.Name, displayResult, resultTruncated, res.Error)
	content := toolResultString(res.Output, res.Error)

	return executedToolStep{
		message: OutputMessage{
			Role:       "tool",
			Content:    content,
			Visible:    opts.toolVisible,
			ToolCallID: call.ToolCallID,
			ToolName:   call.Name,
			ToolArgs:   call.Args,
			ToolResult: payload,
			Meta:       cloneMap(opts.messageMeta),
		},
		chat: providerToolMessage{
			role:       "tool",
			content:    content,
			toolCallID: call.ToolCallID,
		},
		execution: exec,
		action:    action,
	}
}

func (e llmToolExecutor) call(ctx context.Context, event model.InternalEvent, call model.ToolCall, allowedTools map[string]struct{}, counts map[string]int) tools.Result {
	switch {
	case !toolAllowed(call.Name, allowedTools):
		return tools.Result{Error: &model.ErrorBlock{
			Code:    model.ErrorCodeForbidden,
			Message: fmt.Sprintf("tool %q is not allowed for payload_type=%s", call.Name, event.Payload.Type),
		}}
	case strings.EqualFold(strings.TrimSpace(event.Payload.Type), "cron_fire"):
		if errBlock := cronFireToolPolicyError(call, counts); errBlock != nil {
			return tools.Result{Error: errBlock}
		}
		res := callToolSafely(ctx, e.registry, tools.Context{
			Workspace:    e.workspace,
			Conversation: event.Conversation,
		}, call.Name, call.Args)
		counts[call.Name]++
		return res
	default:
		return callToolSafely(ctx, e.registry, tools.Context{
			Workspace:    e.workspace,
			Conversation: event.Conversation,
		}, call.Name, call.Args)
	}
}
