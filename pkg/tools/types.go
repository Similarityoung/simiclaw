package tools

import (
	"context"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

type Context struct {
	Workspace    string
	Scopes       []string
	Conversation model.Conversation
}

type Result struct {
	Disabled bool              `json:"disabled"`
	Output   map[string]any    `json:"output,omitempty"`
	Error    *model.ErrorBlock `json:"error,omitempty"`
}

type Handler func(ctx context.Context, toolCtx Context, args map[string]any) Result
