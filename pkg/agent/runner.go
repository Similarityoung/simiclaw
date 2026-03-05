// Package agent provides AgentRunner: an LLM-backed implementation of
// runner.Runner that uses OpenAI Function Calling to drive multi-step
// tool-use conversations.
//
// Architecture (渐进式方案 C):
//
//	AgentRunner.Run()
//	  ├─ 1. Context Assembler  — load recent session history → []llm.Message
//	  ├─ 2. Build tool list    — tools.Registry.Definitions() → []llm.Tool
//	  ├─ 3. LLM call loop      — up to maxToolRounds tool-calling rounds
//	  │     ├─ call llm.Client.Chat()
//	  │     ├─ if finish_reason == "tool_calls": execute tools, append results
//	  │     └─ else: done
//	  └─ 4. Assemble RunOutput — entries, trace, outbound body
//
// NoReply events (memory_flush, compaction, cron_fire) are handled without
// an LLM call, identical to ProcessRunner's behaviour.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync/atomic"
	"time"

	runner "github.com/similarityyoung/simiclaw/pkg/engine"
	"github.com/similarityyoung/simiclaw/pkg/llm"
	"github.com/similarityyoung/simiclaw/pkg/logging"
	"github.com/similarityyoung/simiclaw/pkg/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
	"github.com/similarityyoung/simiclaw/pkg/tools"
)

// ---- ID generation (identical pattern to ProcessRunner) ---------------------

var idSeq atomic.Uint64

func nextID(prefix string, now time.Time) string {
	n := idSeq.Add(1)
	return fmt.Sprintf("%s_%d_%04d", prefix, now.UnixNano(), n)
}

// ---- AgentRunner ------------------------------------------------------------

// AgentRunner implements runner.Runner using an LLM with Function Calling.
type AgentRunner struct {
	workspace string
	llm       *llm.Client
	registry  *tools.Registry
	writer    *memory.Writer
	// historyLimit is the maximum number of recent session entries loaded into
	// the LLM context window as conversation history.
	historyLimit int
}

// Config configures an AgentRunner.
type Config struct {
	Workspace    string
	LLM          *llm.Client
	Registry     *tools.Registry
	HistoryLimit int // default 40
}

// New creates an AgentRunner.  registry must not be nil.
func New(cfg Config) *AgentRunner {
	limit := cfg.HistoryLimit
	if limit <= 0 {
		limit = 40
	}
	return &AgentRunner{
		workspace:    cfg.Workspace,
		llm:          cfg.LLM,
		registry:     cfg.Registry,
		writer:       memory.NewWriter(cfg.Workspace),
		historyLimit: limit,
	}
}

// ---- runner.Runner implementation -------------------------------------------

// Run processes one InternalEvent and returns a RunOutput.
// It satisfies the runner.Runner interface defined in pkg/engine.
func (r *AgentRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (runner.RunOutput, error) {
	now := time.Now().UTC()
	runID := nextID("run", now)

	mode := model.RunModeNormal
	if isNoReplyPayload(event.Payload.Type) {
		mode = model.RunModeNoReply
	}

	entries := []model.SessionEntry{}
	actions := []model.Action{}
	toolExecutions := []model.ToolExecution{}
	ragHits := []model.RAGHit{}
	outbound := ""

	// user entry (always recorded, even for no-reply)
	entryType := "user"
	content := event.Payload.Text
	if mode == model.RunModeNoReply {
		entryType = "system"
		if content == "" {
			content = event.Payload.Type
		}
	}
	entries = append(entries, model.SessionEntry{
		Type:    entryType,
		EntryID: nextID("e_user", now),
		RunID:   runID,
		Content: content,
	})

	if mode == model.RunModeNoReply {
		r.handleNoReply(event, runID, now, &entries, &actions)
	} else {
		outbound = r.handleInteractive(ctx, event, runID, now, maxToolRounds,
			&entries, &actions, &toolExecutions, &ragHits)
	}

	trace := model.RunTrace{
		RunID:      runID,
		EventID:    event.EventID,
		SessionKey: event.SessionKey,
		SessionID:  event.ActiveSessionID,
		RunMode:    mode,
		ContextManifest: &model.ContextManifest{
			HistoryRange: r.buildHistoryRange(event.ActiveSessionID),
		},
		RAGHits:        ragHits,
		ToolExecutions: toolExecutions,
		Actions:        actions,
		StartedAt:      now,
		FinishedAt:     time.Now().UTC(),
	}

	return runner.RunOutput{
		RunID:          runID,
		RunMode:        mode,
		Entries:        entries,
		Trace:          trace,
		OutboundBody:   outbound,
		SuppressOutput: mode == model.RunModeNoReply,
	}, nil
}

// ---- Interactive handling (LLM + Tool-Calling Loop) -------------------------

func (r *AgentRunner) handleInteractive(
	ctx context.Context,
	event model.InternalEvent,
	runID string,
	now time.Time,
	maxToolRounds int,
	entries *[]model.SessionEntry,
	actions *[]model.Action,
	toolExecutions *[]model.ToolExecution,
	_ *[]model.RAGHit,
) string {
	// 1. Assemble conversation history from session .jsonl
	messages := r.assembleContext(event, now)

	// 2. Build tool list from registry
	llmTools := r.buildToolList()

	logger := logging.L("agent").With(
		logging.String("run_id", runID),
		logging.String("event_id", event.EventID),
		logging.String("session_id", event.ActiveSessionID),
	)

	logger.Info("agent.run.start",
		logging.Int("history_msgs", len(messages)),
		logging.Int("tools", len(llmTools)),
		logging.Int("max_rounds", maxToolRounds),
	)

	// 3. Tool-calling loop
	for round := 0; round <= maxToolRounds; round++ {
		req := llm.ChatRequest{
			Messages: messages,
		}
		if len(llmTools) > 0 && round < maxToolRounds {
			req.Tools = llmTools
		}

		callStart := time.Now()
		resp, err := r.llm.Chat(ctx, req)
		if err != nil {
			errMsg := fmt.Sprintf("LLM 调用失败: %v", err)
			logger.Error("agent.llm.error",
				logging.Int("round", round),
				logging.String("error", err.Error()),
			)
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: errMsg,
			})
			*actions = append(*actions, newSendAction(runID, len(*actions), now, errMsg))
			return errMsg
		}

		if len(resp.Choices) == 0 {
			errMsg := "LLM 返回空响应"
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: errMsg,
			})
			*actions = append(*actions, newSendAction(runID, len(*actions), now, errMsg))
			return errMsg
		}

		choice := resp.Choices[0]
		assistantMsg := choice.Message

		logger.Info("agent.llm.ok",
			logging.Int("round", round),
			logging.String("finish_reason", choice.FinishReason),
			logging.Int("prompt_tokens", resp.Usage.PromptTokens),
			logging.Int("completion_tokens", resp.Usage.CompletionTokens),
			logging.Int64("latency_ms", time.Since(callStart).Milliseconds()),
		)

		if choice.FinishReason == "tool_calls" && len(assistantMsg.ToolCalls) > 0 {
			// Record assistant tool-call entry
			sessionToolCalls := make([]model.ToolCall, 0, len(assistantMsg.ToolCalls))
			for _, tc := range assistantMsg.ToolCalls {
				argsMap := parseArgs(tc.Function.Arguments)
				sessionToolCalls = append(sessionToolCalls, model.ToolCall{
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Args:       argsMap,
				})
			}
			*entries = append(*entries, model.SessionEntry{
				Type:      "assistant",
				EntryID:   nextID("e_assistant_tool", now),
				RunID:     runID,
				ToolCalls: sessionToolCalls,
			})

			// Append assistant message to history for next round
			messages = append(messages, assistantMsg)

			// Execute each tool call
			for _, tc := range assistantMsg.ToolCalls {
				argsMap := parseArgs(tc.Function.Arguments)
				logger.Info("agent.tool.call",
					logging.String("tool", tc.Function.Name),
					logging.String("tool_call_id", tc.ID),
				)
				toolResult := r.executeTool(ctx, event, runID, now, tc.ID, tc.Function.Name, argsMap,
					entries, toolExecutions, actions)
				// Append tool result to history
				messages = append(messages, llm.Message{
					Role:       "tool",
					ToolCallID: tc.ID,
					Name:       tc.Function.Name,
					Content:    toolResult,
				})
			}
			// Continue loop for next LLM call
			continue
		}

		// Terminal response — assistant produced a text reply
		reply := assistantMsg.Content
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		*actions = append(*actions, newSendAction(runID, len(*actions), now, reply))
		logger.Info("agent.run.done",
			logging.Int("rounds", round+1),
			logging.Int("reply_len", len(reply)),
		)

		return reply
	}

	// maxToolRounds exhausted — ask LLM for a final answer without tools
	resp, err := r.llm.Chat(ctx, llm.ChatRequest{Messages: messages})
	if err != nil {
		fallback := "工具调用轮次已达上限，且最终 LLM 调用失败。"
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: fallback,
		})
		return fallback
	}
	reply := ""
	if len(resp.Choices) > 0 {
		reply = resp.Choices[0].Message.Content
	}
	*entries = append(*entries, model.SessionEntry{
		Type:    "assistant",
		EntryID: nextID("e_assistant", now),
		RunID:   runID,
		Content: reply,
	})
	*actions = append(*actions, newSendAction(runID, len(*actions), now, reply))
	return reply
}

// ---- Context Assembler ------------------------------------------------------

// assembleContext builds the []llm.Message slice that forms the LLM prompt.
// It loads recent session entries as history and appends the current user message.
func (r *AgentRunner) assembleContext(event model.InternalEvent, _ time.Time) []llm.Message {
	var messages []llm.Message

	// Load session history
	if event.ActiveSessionID != "" {
		rows, err := store.ReadJSONLines[map[string]any](
			filepath.Join(r.workspace, "runtime", "sessions", event.ActiveSessionID+".jsonl"),
		)
		if err == nil {
			// Take the tail up to historyLimit entries
			start := 0
			if len(rows) > r.historyLimit {
				start = len(rows) - r.historyLimit
			}
			for _, row := range rows[start:] {
				msg := sessionRowToMessage(row)
				if msg != nil {
					messages = append(messages, *msg)
				}
			}
		}
	}

	// Append current user message
	messages = append(messages, llm.Message{
		Role:    "user",
		Content: event.Payload.Text,
	})

	return messages
}

// sessionRowToMessage converts a raw session JSONL row into an llm.Message.
// Rows that don't map to a conversational message (commit markers, memory
// entries, etc.) return nil and are skipped.
func sessionRowToMessage(row map[string]any) *llm.Message {
	t, _ := row["type"].(string)
	content, _ := row["content"].(string)

	switch t {
	case "user":
		return &llm.Message{Role: "user", Content: content}
	case "assistant":
		// May include tool_calls
		if rawTCs, ok := row["tool_calls"]; ok && rawTCs != nil {
			b, _ := json.Marshal(rawTCs)
			var tcs []model.ToolCall
			if json.Unmarshal(b, &tcs) == nil && len(tcs) > 0 {
				llmTCs := make([]llm.ToolCall, 0, len(tcs))
				for _, tc := range tcs {
					argsBytes, _ := json.Marshal(tc.Args)
					llmTCs = append(llmTCs, llm.ToolCall{
						ID:   tc.ToolCallID,
						Type: "function",
						Function: llm.FunctionCall{
							Name:      tc.Name,
							Arguments: string(argsBytes),
						},
					})
				}
				return &llm.Message{Role: "assistant", ToolCalls: llmTCs}
			}
		}
		if content != "" {
			return &llm.Message{Role: "assistant", Content: content}
		}
		return nil
	case "tool_result":
		tcID, _ := row["tool_call_id"].(string)
		result, _ := row["result"].(string)
		return &llm.Message{Role: "tool", ToolCallID: tcID, Content: result}
	default:
		return nil
	}
}

// ---- Tool list builder -------------------------------------------------------

// buildToolList converts the registry's Definitions into llm.Tool entries
// for the OpenAI Function Calling API.
func (r *AgentRunner) buildToolList() []llm.Tool {
	defs := r.registry.Definitions()
	out := make([]llm.Tool, 0, len(defs))
	for _, d := range defs {
		s := d.Schema
		// Convert tools.ParameterSchema → map[string]any for the LLM API
		paramsJSON, _ := json.Marshal(s.Parameters)
		var paramsMap map[string]any
		_ = json.Unmarshal(paramsJSON, &paramsMap)

		out = append(out, llm.Tool{
			Type: "function",
			Function: llm.ToolFunction{
				Name:        s.Name,
				Description: s.Description,
				Parameters:  paramsMap,
			},
		})
	}
	return out
}

// ---- Tool executor ----------------------------------------------------------

// executeTool calls a tool from the registry and records the result in entries
// and toolExecutions.  It returns the result as a JSON string for the LLM.
func (r *AgentRunner) executeTool(
	ctx context.Context,
	event model.InternalEvent,
	runID string,
	now time.Time,
	toolCallID string,
	name string,
	args map[string]any,
	entries *[]model.SessionEntry,
	toolExecutions *[]model.ToolExecution,
	actions *[]model.Action,
) string {
	res := r.registry.Call(ctx, tools.Context{
		Workspace:    r.workspace,
		Scopes:       event.Scopes,
		Conversation: event.Conversation,
	}, name, args)

	// Build result payload JSON (same shape as ProcessRunner)
	resultPayload := map[string]any{"disabled": res.Disabled}
	if res.Output != nil {
		resultPayload["result"] = res.Output
	}
	if res.Error != nil {
		resultPayload["error"] = res.Error
	}
	resultJSON, _ := json.Marshal(resultPayload)

	// Record tool_result entry
	*entries = append(*entries, model.SessionEntry{
		Type:       "tool_result",
		EntryID:    nextID("e_tool_result", now),
		RunID:      runID,
		ToolCallID: toolCallID,
		OK:         res.Error == nil,
		Result:     string(resultJSON),
	})

	// Record action
	*actions = append(*actions, model.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          len(*actions),
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, len(*actions)),
		Type:                 "Invoke",
		Risk:                 "low",
		RequiresApproval:     false,
		Payload: map[string]any{
			"target": "tool",
			"name":   name,
			"args":   args,
		},
	})

	// Record ToolExecution for trace
	*toolExecutions = append(*toolExecutions, model.ToolExecution{
		ToolCallID: toolCallID,
		Name:       name,
		Args:       args,
		Result:     res.Output,
		Error:      res.Error,
	})

	return string(resultJSON)
}

// ---- NoReply handling (reuses ProcessRunner logic verbatim) -----------------

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}

func (r *AgentRunner) handleNoReply(
	event model.InternalEvent,
	runID string,
	now time.Time,
	entries *[]model.SessionEntry,
	actions *[]model.Action,
) {
	switch event.Payload.Type {
	case "memory_flush", "cron_fire":
		note := event.Payload.Text
		if note == "" {
			note = fmt.Sprintf("maintenance event: %s", event.Payload.Type)
		}
		if path, err := r.writer.WriteDaily("system:"+event.Payload.Type, note, now); err == nil {
			*actions = append(*actions, newWriteMemoryAction(runID, len(*actions), now, "daily", path, event.Payload.Type))
			*entries = append(*entries, model.SessionEntry{
				Type:    "memory",
				EntryID: nextID("e_memory", now),
				RunID:   runID,
				Content: note,
				Meta:    map[string]any{"target": "daily", "path": path},
			})
		}
	case "compaction":
		summary, cutoffCommitID := r.buildCompactionSummary(event.ActiveSessionID)
		if isPublicConversation(event.Conversation.ChannelType) {
			if path, err := r.writer.WriteCurated(summary, now); err == nil {
				*actions = append(*actions, newWriteMemoryAction(runID, len(*actions), now, "curated", path, "compaction"))
			}
		}
		*entries = append(*entries, model.SessionEntry{
			Type:    "compaction_summary",
			EntryID: nextID("e_compaction", now),
			RunID:   runID,
			Content: summary,
			Meta: map[string]any{
				"cutoff_commit_id": cutoffCommitID,
			},
		})
	}
}

func isPublicConversation(channelType string) bool {
	switch channelType {
	case "group", "channel":
		return true
	default:
		return false
	}
}

// ---- History / Compaction helpers -------------------------------------------

func (r *AgentRunner) buildHistoryRange(sessionID string) model.HistoryRange {
	rangeInfo := model.HistoryRange{Mode: "tail", TailLimit: r.historyLimit}
	rows, err := store.ReadJSONLines[map[string]any](
		filepath.Join(r.workspace, "runtime", "sessions", sessionID+".jsonl"),
	)
	if err != nil {
		return rangeInfo
	}
	for i := len(rows) - 1; i >= 0; i-- {
		typeName, _ := rows[i]["type"].(string)
		if typeName == "compaction_summary" {
			if meta, ok := rows[i]["meta"].(map[string]any); ok {
				if cutoff, ok := meta["cutoff_commit_id"].(string); ok {
					rangeInfo.CutoffCommitID = cutoff
				}
			}
			break
		}
		if typeName == "commit" && rangeInfo.CutoffCommitID == "" {
			if commit, ok := rows[i]["commit"].(map[string]any); ok {
				if cutoff, ok := commit["commit_id"].(string); ok {
					rangeInfo.CutoffCommitID = cutoff
				}
			}
		}
	}
	return rangeInfo
}

func (r *AgentRunner) buildCompactionSummary(sessionID string) (string, string) {
	rows, err := store.ReadJSONLines[map[string]any](
		filepath.Join(r.workspace, "runtime", "sessions", sessionID+".jsonl"),
	)
	if err != nil || len(rows) == 0 {
		return "compaction: no recent conversational entries", ""
	}
	cutoff := ""
	recent := make([]string, 0, 6)
	for i := len(rows) - 1; i >= 0; i-- {
		typeName, _ := rows[i]["type"].(string)
		if cutoff == "" && typeName == "commit" {
			if commit, ok := rows[i]["commit"].(map[string]any); ok {
				if cid, ok := commit["commit_id"].(string); ok {
					cutoff = cid
				}
			}
		}
		if len(recent) >= 6 {
			continue
		}
		switch typeName {
		case "user", "assistant", "memory":
			c, _ := rows[i]["content"].(string)
			if c != "" {
				recent = append(recent, c)
			}
		}
	}
	if len(recent) == 0 {
		return "compaction: no recent conversational entries", cutoff
	}
	// reverse
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	summary := "compaction summary: "
	for i, s := range recent {
		if i > 0 {
			summary += " | "
		}
		summary += s
	}
	return summary, cutoff
}

// ---- Helpers ----------------------------------------------------------------

func parseArgs(raw string) map[string]any {
	var m map[string]any
	if raw == "" {
		return map[string]any{}
	}
	_ = json.Unmarshal([]byte(raw), &m)
	if m == nil {
		return map[string]any{}
	}
	return m
}

func newSendAction(runID string, actionIndex int, now time.Time, text string) model.Action {
	return model.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          actionIndex,
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, actionIndex),
		Type:                 "SendMessage",
		Risk:                 "low",
		RequiresApproval:     false,
		Payload:              map[string]any{"text": text},
	}
}

func newWriteMemoryAction(runID string, actionIndex int, now time.Time, target, path, reason string) model.Action {
	payload := map[string]any{"target": target, "path": path}
	if reason != "" {
		payload["reason"] = reason
	}
	return model.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          actionIndex,
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, actionIndex),
		Type:                 "WriteMemory",
		Risk:                 "low",
		RequiresApproval:     false,
		Payload:              payload,
	}
}
