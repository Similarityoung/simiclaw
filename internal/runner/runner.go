package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/internal/memory"
	"github.com/similarityyoung/simiclaw/internal/provider"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

type Runner interface {
	Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error)
}

type OutputMessage struct {
	Role       string
	Content    string
	Visible    bool
	ToolCallID string
	ToolName   string
	ToolArgs   map[string]any
	ToolResult map[string]any
	Meta       map[string]any
}

type RunOutput struct {
	RunMode        model.RunMode
	Messages       []OutputMessage
	Trace          model.RunTrace
	AssistantReply string
	SuppressOutput bool
}

type ProviderRunner struct {
	workspace    string
	db           *store.DB
	registry     *tools.Registry
	providers    *provider.Factory
	writer       *memory.Writer
	historyLimit int
}

var idSeq atomic.Uint64

func NewProviderRunner(workspace string, db *store.DB, registry *tools.Registry, providers *provider.Factory) *ProviderRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	return &ProviderRunner{
		workspace:    workspace,
		db:           db,
		registry:     registry,
		providers:    providers,
		writer:       memory.NewWriter(workspace),
		historyLimit: 20,
	}
}

func (r *ProviderRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error) {
	start := time.Now().UTC()
	runMode := model.RunModeNormal
	if isNoReplyPayload(event.Payload.Type) {
		runMode = model.RunModeNoReply
	}
	trace := model.RunTrace{
		EventID:    event.EventID,
		SessionKey: event.SessionKey,
		SessionID:  event.ActiveSessionID,
		RunMode:    runMode,
		Status:     model.RunStatusCompleted,
		StartedAt:  start,
	}
	if runMode == model.RunModeNoReply {
		return r.runNoReply(event, start, trace)
	}
	return r.runInteractive(ctx, event, maxToolRounds, start, trace)
}

func (r *ProviderRunner) runNoReply(event model.InternalEvent, now time.Time, trace model.RunTrace) (RunOutput, error) {
	note := strings.TrimSpace(event.Payload.Text)
	if note == "" {
		note = event.Payload.Type
	}
	switch event.Payload.Type {
	case "memory_flush", "cron_fire":
		_, _ = r.writer.WriteDaily("system:"+event.Payload.Type, note, now)
	case "compaction":
		_, _ = r.writer.WriteCurated(note, now)
	}
	trace.OutputText = note
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()
	return RunOutput{
		RunMode: runModeForPayload(event.Payload.Type),
		Messages: []OutputMessage{{
			Role:    "system",
			Content: note,
			Visible: false,
			Meta:    map[string]any{"payload_type": event.Payload.Type},
		}},
		Trace:          trace,
		AssistantReply: "",
		SuppressOutput: true,
	}, nil
}

func (r *ProviderRunner) runInteractive(ctx context.Context, event model.InternalEvent, maxToolRounds int, now time.Time, trace model.RunTrace) (RunOutput, error) {
	defaultModel := r.providers.DefaultModel()
	llmProvider, actualModel, err := r.providers.Resolve(defaultModel)
	if err != nil {
		trace.Status = model.RunStatusFailed
		trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		trace.FinishedAt = time.Now().UTC()
		trace.LatencyMS = time.Since(now).Milliseconds()
		return RunOutput{RunMode: model.RunModeNormal, Trace: trace}, err
	}

	history, err := r.db.RecentMessages(ctx, event.ActiveSessionID, r.historyLimit)
	if err != nil {
		trace.Status = model.RunStatusFailed
		trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
		trace.FinishedAt = time.Now().UTC()
		trace.LatencyMS = time.Since(now).Milliseconds()
		return RunOutput{RunMode: model.RunModeNormal, Trace: trace}, err
	}
	ragHits, _ := r.db.SearchMessagesFTS(ctx, event.ActiveSessionID, strings.TrimSpace(event.Payload.Text), 5)
	trace.ContextManifest = &model.ContextManifest{
		HistoryRange: model.HistoryRange{Mode: "tail", TailLimit: r.historyLimit},
	}
	trace.RAGHits = ragHits

	messages := []OutputMessage{{
		Role:    "user",
		Content: strings.TrimSpace(event.Payload.Text),
		Visible: true,
	}}
	chatMessages := make([]provider.ChatMessage, 0, len(history)+1)
	for _, msg := range history {
		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		})
	}
	chatMessages = append(chatMessages, provider.ChatMessage{Role: "user", Content: strings.TrimSpace(event.Payload.Text)})

	toolDefs := make([]provider.ToolDefinition, 0)
	for _, def := range r.registry.Definitions() {
		toolDefs = append(toolDefs, provider.ToolDefinition{
			Name:        def.Schema.Name,
			Description: def.Schema.Description,
			Parameters:  def.Schema.Parameters,
		})
	}

	var (
		totalUsage provider.Usage
		reply      string
		last       provider.ChatResult
	)
	for round := 0; round <= maxToolRounds; round++ {
		last, err = llmProvider.Chat(ctx, provider.ChatRequest{
			Model:    actualModel,
			Messages: chatMessages,
			Tools:    toolDefs,
		})
		if err != nil {
			trace.Status = model.RunStatusFailed
			trace.Error = &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: err.Error()}
			trace.FinishedAt = time.Now().UTC()
			trace.LatencyMS = time.Since(now).Milliseconds()
			return RunOutput{
				RunMode:  model.RunModeNormal,
				Messages: messages,
				Trace:    trace,
			}, err
		}
		totalUsage.PromptTokens += last.Usage.PromptTokens
		totalUsage.CompletionTokens += last.Usage.CompletionTokens
		totalUsage.TotalTokens += last.Usage.TotalTokens
		if len(last.ToolCalls) == 0 {
			reply = strings.TrimSpace(last.Text)
			break
		}
		if round == maxToolRounds {
			reply = strings.TrimSpace(last.Text)
			if reply == "" {
				reply = "工具调用轮次已达上限。"
			}
			break
		}

		chatMessages = append(chatMessages, provider.ChatMessage{
			Role:      "assistant",
			ToolCalls: last.ToolCalls,
		})
		for _, call := range last.ToolCalls {
			res := r.registry.Call(ctx, tools.Context{
				Workspace:    r.workspace,
				Conversation: event.Conversation,
			}, call.Name, call.Args)
			exec := model.ToolExecution{
				ToolCallID: call.ToolCallID,
				Name:       call.Name,
				Args:       call.Args,
				Result:     res.Output,
				Error:      res.Error,
			}
			trace.ToolExecutions = append(trace.ToolExecutions, exec)
			trace.Actions = append(trace.Actions, model.Action{
				ActionID:             nextID("act", now),
				ActionIndex:          len(trace.Actions),
				ActionIdempotencyKey: fmt.Sprintf("%s:%d", event.EventID, len(trace.Actions)),
				Type:                 "InvokeTool",
				Risk:                 "low",
				Payload:              map[string]any{"tool_name": call.Name},
			})
			payload := map[string]any{}
			if res.Output != nil {
				payload = res.Output
			}
			content := toolResultString(res.Output, res.Error)
			messages = append(messages, OutputMessage{
				Role:       "tool",
				Content:    content,
				Visible:    true,
				ToolCallID: call.ToolCallID,
				ToolName:   call.Name,
				ToolArgs:   call.Args,
				ToolResult: payload,
			})
			chatMessages = append(chatMessages, provider.ChatMessage{
				Role:       "tool",
				Content:    content,
				ToolCallID: call.ToolCallID,
			})
		}
	}

	if reply == "" {
		reply = strings.TrimSpace(last.Text)
	}
	if reply != "" {
		messages = append(messages, OutputMessage{
			Role:    "assistant",
			Content: reply,
			Visible: true,
		})
	}

	trace.Provider = last.Provider
	trace.Model = last.Model
	trace.PromptTokens = totalUsage.PromptTokens
	trace.CompletionTokens = totalUsage.CompletionTokens
	trace.TotalTokens = totalUsage.TotalTokens
	trace.FinishReason = last.FinishReason
	trace.RawFinishReason = last.RawFinishReason
	trace.ProviderRequestID = last.ProviderRequestID
	trace.OutputText = reply
	trace.ToolCalls = last.ToolCalls
	trace.FinishedAt = time.Now().UTC()
	trace.LatencyMS = time.Since(now).Milliseconds()

	return RunOutput{
		RunMode:        model.RunModeNormal,
		Messages:       messages,
		Trace:          trace,
		AssistantReply: reply,
		SuppressOutput: false,
	}, nil
}

func nextID(prefix string, now time.Time) string {
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}

func toolResultString(output map[string]any, apiErr *model.ErrorBlock) string {
	if apiErr != nil {
		return apiErr.Code + ": " + apiErr.Message
	}
	if len(output) == 0 {
		return "{}"
	}
	b, err := json.Marshal(output)
	if err != nil {
		return "{}"
	}
	return string(b)
}

func runModeForPayload(payloadType string) model.RunMode {
	if isNoReplyPayload(payloadType) {
		return model.RunModeNoReply
	}
	return model.RunModeNormal
}

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}
