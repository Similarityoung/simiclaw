package tools

import (
	"context"

	"github.com/similarityyoung/simiclaw/internal/workspacefile"
	"github.com/similarityyoung/simiclaw/pkg/model"
)

func RegisterWorkspacePatch(reg *Registry) {
	schema := Schema{
		Name:        "workspace_patch",
		Description: "在 workspace 内精确替换文本，或在显式 create=true 时创建新的 UTF-8 文本文件。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"path":     {Type: "string", Description: "文件路径，相对于 workspace"},
				"old_text": {Type: "string", Description: "现有文件中必须恰好命中一次的旧文本；创建新文件时可留空"},
				"new_text": {Type: "string", Description: "替换后的新文本；创建新文件时作为完整初始内容"},
				"create":   {Type: "boolean", Description: "当目标文件不存在时，是否允许创建新文本文件"},
			},
			Required: []string{"path", "new_text"},
		},
	}
	reg.Register("workspace_patch", schema, func(_ context.Context, toolCtx Context, args map[string]any) Result {
		path, _ := args["path"].(string)
		oldText, _ := args["old_text"].(string)
		newText, _ := args["new_text"].(string)
		create, _ := args["create"].(bool)

		res, err := workspacefile.Patch(toolCtx.Workspace, toolCtx.Conversation.ChannelType, workspacefile.PatchArgs{
			Path:    path,
			OldText: oldText,
			NewText: newText,
			Create:  create,
		})
		if err != nil {
			return Result{Error: workspaceFileErrorBlock("workspace_patch", err)}
		}
		return Result{Output: map[string]any{
			"path":          res.Path,
			"operation":     res.Operation,
			"bytes_written": res.BytesWritten,
			"sha256":        res.SHA256,
		}}
	})
}

func RegisterWorkspaceDelete(reg *Registry) {
	schema := Schema{
		Name:        "workspace_delete",
		Description: "删除 workspace 内的单个常规文本文件。",
		Parameters: ParameterSchema{
			Type: "object",
			Properties: map[string]ParameterSchema{
				"path": {Type: "string", Description: "文件路径，相对于 workspace"},
			},
			Required: []string{"path"},
		},
	}
	reg.Register("workspace_delete", schema, func(_ context.Context, toolCtx Context, args map[string]any) Result {
		path, _ := args["path"].(string)
		res, err := workspacefile.Delete(toolCtx.Workspace, toolCtx.Conversation.ChannelType, workspacefile.DeleteArgs{Path: path})
		if err != nil {
			return Result{Error: workspaceFileErrorBlock("workspace_delete", err)}
		}
		return Result{Output: map[string]any{
			"path":      res.Path,
			"operation": res.Operation,
		}}
	})
}

func workspaceFileErrorBlock(toolName string, err error) *model.ErrorBlock {
	if err == nil {
		return nil
	}
	if toolErr, ok := err.(*workspacefile.Error); ok {
		return &model.ErrorBlock{
			Code:    workspaceFileErrorCode(toolErr.Code),
			Message: toolName + " failed: " + toolErr.Message,
			Details: toolErr.Details,
		}
	}
	return &model.ErrorBlock{Code: model.ErrorCodeInternal, Message: toolName + " failed: " + err.Error()}
}

func workspaceFileErrorCode(code string) string {
	switch code {
	case workspacefile.CodeInvalidArgument:
		return model.ErrorCodeInvalidArgument
	case workspacefile.CodeNotFound:
		return model.ErrorCodeNotFound
	case workspacefile.CodeConflict:
		return model.ErrorCodeConflict
	case workspacefile.CodeForbidden:
		return model.ErrorCodeForbidden
	default:
		return model.ErrorCodeInternal
	}
}
