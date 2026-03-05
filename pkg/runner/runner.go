package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
	"github.com/similarityyoung/simiclaw/pkg/store"
	"github.com/similarityyoung/simiclaw/pkg/tools"
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

type ProcessRunner struct {
	workspace string
	registry  *tools.Registry
	writer    *memory.Writer
	tailLimit int
}

func NewProcessRunner(workspace string, registry *tools.Registry) *ProcessRunner {
	if registry == nil {
		registry = tools.NewRegistry()
		tools.RegisterBuiltins(registry)
	}
	return &ProcessRunner{
		workspace: workspace,
		registry:  registry,
		writer:    memory.NewWriter(workspace),
		tailLimit: 200,
	}
}

func (r *ProcessRunner) Run(ctx context.Context, event model.InternalEvent, maxToolRounds int) (RunOutput, error) {
	now := time.Now().UTC()
	runID := nextID("run", now)

	mode := model.RunModeNormal
	if isNoReplyPayload(event.Payload.Type) {
		mode = model.RunModeNoReply
	}

	entryType := "user"
	content := strings.TrimSpace(event.Payload.Text)
	if mode == model.RunModeNoReply {
		entryType = "system"
		if content == "" {
			content = event.Payload.Type
		}
	}

	entries := []model.SessionEntry{{
		Type:    entryType,
		EntryID: nextID("e_user", now),
		RunID:   runID,
		Content: content,
	}}
	actions := make([]model.Action, 0, 8)
	rawHits := make([]model.RAGHit, 0, 4)
	toolExecutions := make([]model.ToolExecution, 0, 4)
	outbound := ""

	if mode == model.RunModeNoReply {
		r.handleNoReply(event, runID, now, &entries, &actions)
	} else {
		outbound = r.handleInteractive(ctx, event, runID, now, maxToolRounds, &entries, &actions, &toolExecutions, &rawHits)
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
		RAGHits:        rawHits,
		ToolExecutions: toolExecutions,
		Actions:        actions,
		StartedAt:      now,
		FinishedAt:     time.Now().UTC(),
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

func isNoReplyPayload(payloadType string) bool {
	return payloadType == "memory_flush" || payloadType == "compaction" || payloadType == "cron_fire"
}

func (r *ProcessRunner) handleNoReply(event model.InternalEvent, runID string, now time.Time, entries *[]model.SessionEntry, actions *[]model.Action) {
	switch event.Payload.Type {
	case "memory_flush", "cron_fire":
		note := strings.TrimSpace(event.Payload.Text)
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
				Meta: map[string]any{
					"target": "daily",
					"path":   path,
				},
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
				"tail_limit":       r.tailLimit,
			},
		})
	}
}

func (r *ProcessRunner) handleInteractive(
	ctx context.Context,
	event model.InternalEvent,
	runID string,
	now time.Time,
	maxToolRounds int,
	entries *[]model.SessionEntry,
	actions *[]model.Action,
	toolExecutions *[]model.ToolExecution,
	ragHits *[]model.RAGHit,
) string {
	text := strings.TrimSpace(event.Payload.Text)

	if strings.HasPrefix(text, "/memory_get ") {
		path, lines := parseMemoryGetCommand(text)
		args := map[string]any{"path": path}
		if len(lines) == 2 {
			args["lines"] = lines
		}
		*actions = append(*actions, newInvokeAction(runID, len(*actions), now, "memory_get", args))
		res := r.invokeTool(ctx, event, runID, now, "memory_get", args, entries, toolExecutions)
		if res.Error != nil {
			reply := "memory_get 被拒绝: " + res.Error.Message
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: reply,
			})
			return reply
		}
		content, _ := res.Output["content"].(string)
		reply := strings.TrimSpace(content)
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		return reply
	}

	if strings.HasPrefix(text, "/patch") {
		patchPayload := map[string]any{}
		for k, v := range event.Payload.Extra {
			patchPayload[k] = v
		}
		if _, ok := patchPayload["target"]; !ok {
			patchPayload["target"] = "workflow"
		}
		if _, ok := patchPayload["patch_format"]; !ok {
			patchPayload["patch_format"] = "unified-diff"
		}
		if _, ok := patchPayload["patch_idempotency_key"]; !ok {
			patchPayload["patch_idempotency_key"] = fmt.Sprintf("%s:act_patch_%d", runID, len(*actions))
		}
		*actions = append(*actions, model.Action{
			ActionID:             nextID("act", now),
			ActionIndex:          len(*actions),
			ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, len(*actions)),
			Type:                 "Patch",
			Risk:                 "high",
			RequiresApproval:     true,
			Payload:              patchPayload,
		})
		reply := "已生成高风险 Patch 提案，等待审批。"
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		return reply
	}

	if shouldRemember(text) {
		fact := extractRememberFact(text)
		reply := "好的，我记住了。"
		if fact != "" {
			reply = "好的，我记住了：" + fact
		}
		if dailyPath, err := r.writer.WriteDaily("user:"+event.Conversation.ParticipantID, fact, now); err == nil {
			*actions = append(*actions, newWriteMemoryAction(runID, len(*actions), now, "daily", dailyPath, "user_fact"))
		}
		if isPublicConversation(event.Conversation.ChannelType) {
			if curatedPath, err := r.writer.WriteCurated(fact, now); err == nil {
				*actions = append(*actions, newWriteMemoryAction(runID, len(*actions), now, "curated", curatedPath, "user_fact"))
			}
		}
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		return reply
	}

	if shouldRecall(text) && maxToolRounds > 0 {
		query := deriveRecallQuery(text)
		searchArgs := map[string]any{"query": query, "scope": "auto", "top_k": 6}
		*actions = append(*actions, newInvokeAction(runID, len(*actions), now, "memory_search", searchArgs))
		searchRes := r.invokeTool(ctx, event, runID, now, "memory_search", searchArgs, entries, toolExecutions)
		if searchRes.Error != nil {
			reply := "我暂时无法读取记忆：" + searchRes.Error.Message
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: reply,
			})
			return reply
		}

		hits := decodeHits(searchRes.Output["hits"])
		*ragHits = append(*ragHits, hits...)
		if len(hits) == 0 {
			reply := "我还没记住相关信息。"
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: reply,
			})
			return reply
		}

		getArgs := map[string]any{"path": hits[0].Path, "lines": hits[0].Lines}
		*actions = append(*actions, newInvokeAction(runID, len(*actions), now, "memory_get", getArgs))
		getRes := r.invokeTool(ctx, event, runID, now, "memory_get", getArgs, entries, toolExecutions)
		if getRes.Error != nil {
			reply := "我找到了线索，但读取片段失败：" + getRes.Error.Message
			*entries = append(*entries, model.SessionEntry{
				Type:    "assistant",
				EntryID: nextID("e_assistant", now),
				RunID:   runID,
				Content: reply,
			})
			return reply
		}
		content, _ := getRes.Output["content"].(string)
		snippet := firstFactLine(content)
		reply := "你之前提到：" + snippet
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant", now),
			RunID:   runID,
			Content: reply,
		})
		return reply
	}

	if strings.Contains(text, "/tool") && maxToolRounds > 0 {
		toolCallID := nextID("tc", now)
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant_tool", now),
			RunID:   runID,
			ToolCalls: []model.ToolCall{{
				ToolCallID: toolCallID,
				Name:       "mock_tool",
				Args:       map[string]any{"query": text},
			}},
		})
		*entries = append(*entries, model.SessionEntry{
			Type:       "tool_result",
			EntryID:    nextID("e_tool_result", now),
			RunID:      runID,
			ToolCallID: toolCallID,
			OK:         true,
			Result:     "tool_result_ok",
		})
		final := "工具执行完成"
		*entries = append(*entries, model.SessionEntry{
			Type:    "assistant",
			EntryID: nextID("e_assistant_final", now),
			RunID:   runID,
			Content: final,
		})
		*actions = append(*actions, model.Action{
			ActionID:             nextID("act", now),
			ActionIndex:          len(*actions),
			ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, len(*actions)),
			Type:                 "Invoke",
			Risk:                 "low",
			RequiresApproval:     false,
			Payload:              map[string]any{"target": "tool", "name": "mock_tool"},
		})
		return final
	}

	reply := "已收到: " + text
	*entries = append(*entries, model.SessionEntry{
		Type:    "assistant",
		EntryID: nextID("e_assistant", now),
		RunID:   runID,
		Content: reply,
	})
	*actions = append(*actions, model.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          len(*actions),
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, len(*actions)),
		Type:                 "SendMessage",
		Risk:                 "low",
		RequiresApproval:     false,
		Payload:              map[string]any{"text": reply},
	})
	return reply
}

func (r *ProcessRunner) invokeTool(
	ctx context.Context,
	event model.InternalEvent,
	runID string,
	now time.Time,
	name string,
	args map[string]any,
	entries *[]model.SessionEntry,
	toolExecutions *[]model.ToolExecution,
) tools.Result {
	if args == nil {
		args = map[string]any{}
	}
	toolCallID := nextID("tc", now)
	*entries = append(*entries, model.SessionEntry{
		Type:    "assistant",
		EntryID: nextID("e_assistant_tool", now),
		RunID:   runID,
		ToolCalls: []model.ToolCall{{
			ToolCallID: toolCallID,
			Name:       name,
			Args:       args,
		}},
	})

	res := r.registry.Call(ctx, tools.Context{
		Workspace:    r.workspace,
		Scopes:       event.Scopes,
		Conversation: event.Conversation,
	}, name, args)

	resultPayload := map[string]any{"disabled": res.Disabled}
	if res.Output != nil {
		resultPayload["result"] = res.Output
	}
	if res.Error != nil {
		resultPayload["error"] = res.Error
	}
	resultJSON, _ := json.Marshal(resultPayload)
	*entries = append(*entries, model.SessionEntry{
		Type:       "tool_result",
		EntryID:    nextID("e_tool_result", now),
		RunID:      runID,
		ToolCallID: toolCallID,
		OK:         res.Error == nil,
		Result:     string(resultJSON),
	})

	*toolExecutions = append(*toolExecutions, model.ToolExecution{
		ToolCallID:  toolCallID,
		Name:        name,
		Args:        args,
		ArgsSummary: summarizeArgs(args),
		Result:      res.Output,
		Error:       res.Error,
	})
	return res
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

func newInvokeAction(runID string, actionIndex int, now time.Time, name string, args map[string]any) model.Action {
	return model.Action{
		ActionID:             nextID("act", now),
		ActionIndex:          actionIndex,
		ActionIdempotencyKey: fmt.Sprintf("%s:%d", runID, actionIndex),
		Type:                 "Invoke",
		Risk:                 "low",
		RequiresApproval:     false,
		Payload: map[string]any{
			"target": "tool",
			"name":   name,
			"args":   args,
		},
	}
}

func summarizeArgs(args map[string]any) map[string]any {
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	summary := map[string]any{"keys": keys}
	if q, ok := args["query"].(string); ok {
		summary["query_len"] = len([]rune(q))
	}
	if lines, ok := args["lines"].([]int); ok && len(lines) == 2 {
		summary["line_span"] = fmt.Sprintf("%d:%d", lines[0], lines[1])
	}
	if raw, ok := args["lines"].([]any); ok && len(raw) == 2 {
		start := fmt.Sprintf("%v", raw[0])
		end := fmt.Sprintf("%v", raw[1])
		summary["line_span"] = start + ":" + end
	}
	if _, ok := args["path"].(string); ok {
		summary["has_path"] = true
	}
	return summary
}

func shouldRemember(text string) bool {
	lower := strings.ToLower(text)
	if strings.HasPrefix(lower, "remember ") {
		return true
	}
	return strings.Contains(text, "记住")
}

func isPublicConversation(channelType string) bool {
	switch strings.ToLower(strings.TrimSpace(channelType)) {
	case "group", "channel":
		return true
	default:
		return false
	}
}

func extractRememberFact(text string) string {
	trimmed := strings.TrimSpace(text)
	if idx := strings.Index(trimmed, "记住"); idx >= 0 {
		fact := strings.TrimSpace(trimmed[idx+len("记住"):])
		fact = strings.TrimLeft(fact, "：:，,。.!！？? ")
		return fact
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "remember ") {
		return strings.TrimSpace(trimmed[len("remember "):])
	}
	return trimmed
}

func shouldRecall(text string) bool {
	lower := strings.ToLower(text)
	if strings.Contains(lower, "what do i like") {
		return true
	}
	return strings.Contains(text, "喜欢什么") || strings.Contains(text, "我喜欢什么") || strings.Contains(text, "记得我")
}

func deriveRecallQuery(text string) string {
	if strings.Contains(text, "喜欢") {
		return "喜欢"
	}
	return strings.TrimSpace(text)
}

func decodeHits(v any) []model.RAGHit {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var hits []model.RAGHit
	if err := json.Unmarshal(b, &hits); err != nil {
		return nil
	}
	return hits
}

func firstFactLine(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "...") {
			continue
		}
		if idx := strings.Index(line, ": "); idx >= 0 {
			candidate := strings.TrimSpace(line[idx+2:])
			if candidate != "" {
				return candidate
			}
		}
		return line
	}
	return "暂无可用片段"
}

func parseMemoryGetCommand(text string) (string, []int) {
	parts := strings.Fields(strings.TrimSpace(text))
	if len(parts) < 2 {
		return "", nil
	}
	path := parts[1]
	if len(parts) < 3 {
		return path, nil
	}
	pair := strings.SplitN(parts[2], ":", 2)
	if len(pair) != 2 {
		return path, nil
	}
	start, err1 := strconv.Atoi(pair[0])
	end, err2 := strconv.Atoi(pair[1])
	if err1 != nil || err2 != nil {
		return path, nil
	}
	return path, []int{start, end}
}

func (r *ProcessRunner) buildHistoryRange(sessionID string) model.HistoryRange {
	rangeInfo := model.HistoryRange{
		Mode:      "compaction+tail",
		TailLimit: r.tailLimit,
	}
	rows, err := store.ReadJSONLines[map[string]any](filepath.Join(r.workspace, "runtime", "sessions", sessionID+".jsonl"))
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

func (r *ProcessRunner) buildCompactionSummary(sessionID string) (string, string) {
	rows, err := store.ReadJSONLines[map[string]any](filepath.Join(r.workspace, "runtime", "sessions", sessionID+".jsonl"))
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
			content, _ := rows[i]["content"].(string)
			content = strings.TrimSpace(content)
			if content != "" {
				recent = append(recent, content)
			}
		}
	}

	if len(recent) == 0 {
		return "compaction: no recent conversational entries", cutoff
	}
	for i, j := 0, len(recent)-1; i < j; i, j = i+1, j-1 {
		recent[i], recent[j] = recent[j], recent[i]
	}
	summary := "compaction summary: " + strings.Join(recent, " | ")
	return summary, cutoff
}
