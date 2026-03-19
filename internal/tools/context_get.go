package tools

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/similarityyoung/simiclaw/internal/workspacefile"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterContextGet(reg *Registry) {
	schema := Schema{
		Name:        "context_get",
		Description: "读取工作区根目录上下文文件或 skill 正文，支持指定行范围。",
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

		res, err := workspacefile.GetContext(toolCtx.Workspace, workspacefile.ContextGetArgs{Path: path, Lines: lines}, workspacefile.DefaultMaxContextChars)
		if err != nil {
			return Result{Error: contextGetError(err)}
		}
		return Result{Output: map[string]any{"path": res.Path, "content": res.Content}}
	})
}

func contextGetError(err error) *model.ErrorBlock {
	code := model.ErrorCodeInvalidArgument
	switch {
	case os.IsNotExist(err), errors.Is(err, os.ErrNotExist):
		code = model.ErrorCodeNotFound
	default:
		var toolErr *workspacefile.Error
		if errors.As(err, &toolErr) {
			switch toolErr.Code {
			case workspacefile.CodeNotFound:
				code = model.ErrorCodeNotFound
			case workspacefile.CodeForbidden:
				code = model.ErrorCodeForbidden
			default:
				code = model.ErrorCodeInvalidArgument
			}
		}
	}
	return &model.ErrorBlock{Code: code, Message: fmt.Sprintf("context_get failed: %v", err)}
}
