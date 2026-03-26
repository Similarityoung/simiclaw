package runner

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/runtime/kernel"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

const payloadTypeMetaKey = "payload_type"
const payloadTypeNewSession = "new_session"

var cronFireToolBudgets = map[string]int{
	"memory_search": 1,
	"memory_get":    1,
	"context_get":   1,
}

var cronFireInjectedRootFiles = map[string]struct{}{
	"SOUL.md":      {},
	"IDENTITY.md":  {},
	"USER.md":      {},
	"AGENTS.md":    {},
	"TOOLS.md":     {},
	"BOOTSTRAP.md": {},
	"HEARTBEAT.md": {},
}

func maxToolRoundsReply(last kernel.ModelResult) string {
	if len(last.ToolCalls) > 0 {
		return "工具调用轮次已达上限。"
	}
	reply := strings.TrimSpace(last.Text)
	if reply == "" {
		return "工具调用轮次已达上限。"
	}
	return reply
}

func cronFireToolPolicyError(call model.ToolCall, counts map[string]int) *model.ErrorBlock {
	if budget, ok := cronFireToolBudgets[call.Name]; ok && counts[call.Name] >= budget {
		return &model.ErrorBlock{
			Code:    model.ErrorCodeForbidden,
			Message: fmt.Sprintf("cron_fire tool budget exhausted for %q; summarize with current evidence instead of fetching more context", call.Name),
		}
	}
	if call.Name != "context_get" {
		return nil
	}
	path, _ := call.Args["path"].(string)
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(strings.TrimSpace(path))))
	if _, ok := cronFireInjectedRootFiles[clean]; ok {
		return &model.ErrorBlock{
			Code:    model.ErrorCodeForbidden,
			Message: fmt.Sprintf("context_get %q is already injected into the cron_fire system prompt; summarize with current evidence instead of rereading it", clean),
		}
	}
	return nil
}

func toolAllowed(name string, allowed map[string]struct{}) bool {
	if allowed == nil {
		return true
	}
	_, ok := allowed[name]
	return ok
}

func toolRisk(name string) string {
	switch name {
	case "workspace_patch":
		return "medium"
	case "workspace_delete":
		return "high"
	default:
		return "low"
	}
}
