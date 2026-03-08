package tools

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/similarityyoung/simiclaw/internal/contextfile"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterContextGet(reg *Registry) {
	schema := Schema{
		Name:        "context_get",
		Description: "读取工作区 bootstrap 文件或 skill 正文，支持指定行范围。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"path":  {Type: "string", Description: "文件路径，相对于 workspace"},
				"lines": {Type: "array", Description: "[start, end] 行号范围（可选）", Items: &ParameterSchema{Type: "integer"}},
			},
			Required: []string{"path"},
		},
	}
	reg.Register("context_get", schema, func(_ context.Context, toolCtx Context, args map[string]any) Result {
		path, _ := args["path"].(string)
		lines := parseLines(args["lines"])

		if _, _, err := contextfile.ResolvePath(toolCtx.Workspace, path); err != nil {
			return Result{Error: &model.ErrorBlock{Code: model.ErrorCodeForbidden, Message: fmt.Sprintf("context_get failed: %v", err)}}
		}

		res, err := contextfile.Get(toolCtx.Workspace, contextfile.GetArgs{Path: path, Lines: lines}, contextfile.DefaultMaxGetChars)
		if err != nil {
			code := model.ErrorCodeInvalidArgument
			switch {
			case os.IsNotExist(err):
				code = model.ErrorCodeNotFound
			case strings.Contains(err.Error(), "path denied"):
				code = model.ErrorCodeForbidden
			case errors.Is(err, os.ErrNotExist):
				code = model.ErrorCodeNotFound
			}
			return Result{Error: &model.ErrorBlock{Code: code, Message: fmt.Sprintf("context_get failed: %v", err)}}
		}
		return Result{Output: map[string]any{"path": res.Path, "content": res.Content}}
	})
}
