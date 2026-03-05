package tools

import (
	"context"
	"fmt"

	"github.com/similarityyoung/simiclaw/pkg/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterMemorySearch(reg *Registry) {
	reg.Register("memory_search", func(_ context.Context, toolCtx Context, args map[string]any) Result {
		query, _ := args["query"].(string)
		scope, _ := args["scope"].(string)
		topK := parseInt(args["top_k"], 6)

		res, err := memory.Search(toolCtx.Workspace, memory.SearchArgs{
			Query:       query,
			Scope:       scope,
			TopK:        topK,
			ChannelType: toolCtx.Conversation.ChannelType,
		})
		if err != nil {
			return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeInvalidArgument, Message: fmt.Sprintf("memory_search failed: %v", err)}}
		}
		return Result{Disabled: res.Disabled, Output: map[string]any{"disabled": res.Disabled, "hits": res.Hits}}
	})
}
