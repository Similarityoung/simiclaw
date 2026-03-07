package tools

import (
	"context"
	"fmt"

	"github.com/similarityyoung/simiclaw/internal/memory"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterMemorySearch(reg *Registry) {
	schema := Schema{
		Name:        "memory_search",
		Description: "在工作区记忆中搜索相关信息，返回匹配的片段列表。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"query": {Type: "string", Description: "搜索关键词"},
				"scope": {Type: "string", Description: "auto | daily | curated", Enum: []string{"auto", "daily", "curated"}},
				"top_k": {Type: "integer", Description: "返回条数，默认 6"},
			},
			Required: []string{"query"},
		},
	}
	reg.Register("memory_search", schema, func(_ context.Context, toolCtx Context, args map[string]any) Result {
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
