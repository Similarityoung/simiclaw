package tools

import (
	"context"
	"errors"
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterMemoryGet(reg *Registry) {
	reg.Register("memory_get", func(_ context.Context, toolCtx Context, args map[string]any) Result {
		path, _ := args["path"].(string)
		lines := parseLines(args["lines"])

		_, _, scope, err := memory.ResolvePath(toolCtx.Workspace, path)
		if err != nil {
			code := model.ErrorCodeInvalidArgument
			if errors.Is(err, memory.ErrPathDenied) {
				code = model.ErrorCodeForbidden
			}
			return Result{Error: &model.ErrorBlock{Code: code, Message: fmt.Sprintf("memory_get failed: %v", err)}}
		}
		if !memory.CanAccessScope(toolCtx.Conversation.ChannelType, scope) {
			return Result{
				Error: &model.ErrorBlock{
					Code:    model.ErrorCodeForbidden,
					Message: "memory_get failed: scope denied",
				},
			}
		}

		res, err := memory.Get(toolCtx.Workspace, memory.GetArgs{Path: path, Lines: lines}, memory.DefaultMaxGetChars)
		if err != nil {
			code := model.ErrorCodeInvalidArgument
			if errors.Is(err, memory.ErrPathDenied) {
				code = model.ErrorCodeForbidden
			}
			return Result{Error: &model.ErrorBlock{Code: code, Message: fmt.Sprintf("memory_get failed: %v", err)}}
		}
		return Result{Output: map[string]any{"path": res.Path, "content": res.Content}}
	})
}

func RegisterBuiltins(reg *Registry) {
	RegisterMemorySearch(reg)
	RegisterMemoryGet(reg)
}

func parseInt(v any, fallback int) int {
	switch tv := v.(type) {
	case int:
		return tv
	case int32:
		return int(tv)
	case int64:
		return int(tv)
	case float64:
		return int(tv)
	default:
		return fallback
	}
}

func parseLines(v any) []int {
	raw, ok := v.([]any)
	if !ok {
		if ints, ok := v.([]int); ok {
			return ints
		}
		return nil
	}
	out := make([]int, 0, len(raw))
	for _, item := range raw {
		out = append(out, parseInt(item, 0))
	}
	return out
}
