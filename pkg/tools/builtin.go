package tools

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

const (
	fileReadToolName     = "file_read"
	fileReadDefaultLimit = 2000
	fileWriteToolName    = "file_write"
	fileEditToolName     = "file_edit"
)

var errFileReadPathDenied = errors.New("path denied")

type FileReadInput struct {
	Path   string `json:"path"`
	Offset *int   `json:"offset,omitempty"`
	Limit  *int   `json:"limit,omitempty"`
}

type FileReadOutput struct {
	Path       string `json:"path"`
	Content    string `json:"content"`
	StartLine  int    `json:"start_line,omitempty"`
	EndLine    int    `json:"end_line,omitempty"`
	TotalLines int    `json:"total_lines"`
	Truncated  bool   `json:"truncated"`
}

type FileWriteInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type FileWriteOutput struct {
	Path         string `json:"path"`
	BytesWritten int    `json:"bytes_written"`
}

type FileEditInput struct {
	Path       string `json:"path"`
	OldText    string `json:"old_text"`
	NewText    string `json:"new_text"`
	ReplaceAll bool   `json:"replace_all,omitempty"`
}

type FileEditOutput struct {
	Path          string `json:"path"`
	ReplacedCount int    `json:"replaced_count"`
	BytesWritten  int    `json:"bytes_written"`
}

func NewFileReadTool(workspace string) (adktool.Tool, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("invalid_argument: workspace is required")
	}

	return functiontool.New[FileReadInput, FileReadOutput](functiontool.Config{
		Name:        fileReadToolName,
		Description: "Read text files in workspace with optional line window.",
	}, func(_ adktool.Context, input FileReadInput) (FileReadOutput, error) {
		path, absPath, err := resolveFileReadPath(workspace, input.Path)
		if err != nil {
			if errors.Is(err, errFileReadPathDenied) {
				return FileReadOutput{}, fmt.Errorf("forbidden: %w", err)
			}
			return FileReadOutput{}, fmt.Errorf("invalid_argument: %w", err)
		}

		startLine := 1
		if input.Offset != nil {
			if *input.Offset < 1 {
				return FileReadOutput{}, fmt.Errorf("invalid_argument: offset must be >= 1")
			}
			startLine = *input.Offset
		}

		lineLimit := fileReadDefaultLimit
		if input.Limit != nil {
			if *input.Limit < 1 {
				return FileReadOutput{}, fmt.Errorf("invalid_argument: limit must be >= 1")
			}
			lineLimit = *input.Limit
		}

		out, err := buildFileReadOutput(path, absPath, startLine, lineLimit)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return FileReadOutput{}, fmt.Errorf("not_found: %w", err)
			}
			return FileReadOutput{}, fmt.Errorf("internal: %w", err)
		}
		return out, nil
	})
}

func NewFileWriteTool(workspace string) (adktool.Tool, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("invalid_argument: workspace is required")
	}

	return functiontool.New[FileWriteInput, FileWriteOutput](functiontool.Config{
		Name:        fileWriteToolName,
		Description: "Write text files in workspace safely.",
	}, func(_ adktool.Context, input FileWriteInput) (FileWriteOutput, error) {
		return writeWorkspaceFile(workspace, input)
	})
}

func NewFileEditTool(workspace string) (adktool.Tool, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return nil, fmt.Errorf("invalid_argument: workspace is required")
	}

	return functiontool.New[FileEditInput, FileEditOutput](functiontool.Config{
		Name:        fileEditToolName,
		Description: "Replace text in workspace files with controlled matching.",
	}, func(_ adktool.Context, input FileEditInput) (FileEditOutput, error) {
		return editWorkspaceFile(workspace, input)
	})
}

func writeWorkspaceFile(workspace string, input FileWriteInput) (FileWriteOutput, error) {
	path, absPath, err := resolveFileReadPath(workspace, input.Path)
	if err != nil {
		if errors.Is(err, errFileReadPathDenied) {
			return FileWriteOutput{}, fmt.Errorf("forbidden: %w", err)
		}
		return FileWriteOutput{}, fmt.Errorf("invalid_argument: %w", err)
	}

	if err := ensureParentPathWithinWorkspace(workspace, absPath); err != nil {
		if errors.Is(err, errFileReadPathDenied) {
			return FileWriteOutput{}, fmt.Errorf("forbidden: %w", err)
		}
		return FileWriteOutput{}, fmt.Errorf("internal: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return FileWriteOutput{}, fmt.Errorf("internal: %w", err)
	}

	if err := os.WriteFile(absPath, []byte(input.Content), 0o644); err != nil {
		return FileWriteOutput{}, fmt.Errorf("internal: %w", err)
	}

	return FileWriteOutput{Path: path, BytesWritten: len(input.Content)}, nil
}

func editWorkspaceFile(workspace string, input FileEditInput) (FileEditOutput, error) {
	path, absPath, err := resolveFileReadPath(workspace, input.Path)
	if err != nil {
		if errors.Is(err, errFileReadPathDenied) {
			return FileEditOutput{}, fmt.Errorf("forbidden: %w", err)
		}
		return FileEditOutput{}, fmt.Errorf("invalid_argument: %w", err)
	}

	if input.OldText == "" {
		return FileEditOutput{}, fmt.Errorf("invalid_argument: old_text is required")
	}

	if err := ensureParentPathWithinWorkspace(workspace, absPath); err != nil {
		if errors.Is(err, errFileReadPathDenied) {
			return FileEditOutput{}, fmt.Errorf("forbidden: %w", err)
		}
		return FileEditOutput{}, fmt.Errorf("internal: %w", err)
	}

	content, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return FileEditOutput{}, fmt.Errorf("not_found: %w", err)
		}
		return FileEditOutput{}, fmt.Errorf("internal: %w", err)
	}

	current := string(content)
	matches := strings.Count(current, input.OldText)
	if matches == 0 {
		return FileEditOutput{}, fmt.Errorf("not_found: old_text not found")
	}
	if !input.ReplaceAll && matches > 1 {
		return FileEditOutput{}, fmt.Errorf("invalid_argument: old_text has multiple matches; set replace_all=true")
	}

	next := current
	replaced := matches
	if input.ReplaceAll {
		next = strings.ReplaceAll(current, input.OldText, input.NewText)
	} else {
		next = strings.Replace(current, input.OldText, input.NewText, 1)
		replaced = 1
	}

	if err := os.WriteFile(absPath, []byte(next), 0o644); err != nil {
		return FileEditOutput{}, fmt.Errorf("internal: %w", err)
	}

	return FileEditOutput{Path: path, ReplacedCount: replaced, BytesWritten: len(next)}, nil
}

func resolveFileReadPath(workspace, rawPath string) (string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", fmt.Errorf("%w: empty path", errFileReadPathDenied)
	}

	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", fmt.Errorf("%w: outside workspace", errFileReadPathDenied)
	}

	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}

	absPath, err := filepath.Abs(filepath.Join(workspaceAbs, clean))
	if err != nil {
		return "", "", err
	}

	inside, err := isWithinWorkspace(workspaceAbs, absPath)
	if err != nil {
		return "", "", err
	}
	if !inside {
		return "", "", fmt.Errorf("%w: outside workspace", errFileReadPathDenied)
	}

	if resolved, err := filepath.EvalSymlinks(absPath); err == nil {
		resolvedAbs, err := filepath.Abs(resolved)
		if err != nil {
			return "", "", err
		}
		inside, err = isWithinWorkspace(workspaceReal, resolvedAbs)
		if err != nil {
			return "", "", err
		}
		if !inside {
			return "", "", fmt.Errorf("%w: symlink escapes workspace", errFileReadPathDenied)
		}
	} else if !os.IsNotExist(err) {
		return "", "", err
	}

	return filepath.ToSlash(clean), absPath, nil
}

func isWithinWorkspace(workspaceAbs, candidateAbs string) (bool, error) {
	relCheck, err := filepath.Rel(workspaceAbs, candidateAbs)
	if err != nil {
		return false, err
	}
	if relCheck == ".." || strings.HasPrefix(relCheck, ".."+string(os.PathSeparator)) {
		return false, nil
	}
	return true, nil
}

func ensureParentPathWithinWorkspace(workspace, absPath string) error {
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}

	parent := filepath.Dir(absPath)
	for {
		if parent == "." || parent == string(os.PathSeparator) {
			break
		}
		_, statErr := os.Stat(parent)
		if statErr == nil {
			resolvedParent, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return err
			}
			resolvedParentAbs, err := filepath.Abs(resolvedParent)
			if err != nil {
				return err
			}
			inside, err := isWithinWorkspace(workspaceReal, resolvedParentAbs)
			if err != nil {
				return err
			}
			if !inside {
				return fmt.Errorf("%w: symlink escapes workspace", errFileReadPathDenied)
			}
			return nil
		}
		if !os.IsNotExist(statErr) {
			return statErr
		}

		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}

	return nil
}

func buildFileReadOutput(path, absPath string, startLine, lineLimit int) (FileReadOutput, error) {
	content, err := os.ReadFile(absPath)
	if err != nil {
		return FileReadOutput{}, err
	}

	lines := splitLines(string(content))
	total := len(lines)

	if startLine > total || total == 0 {
		return FileReadOutput{
			Path:       path,
			Content:    "",
			TotalLines: total,
			Truncated:  false,
		}, nil
	}

	start := startLine - 1
	end := start + lineLimit
	if end > total {
		end = total
	}

	return FileReadOutput{
		Path:       path,
		Content:    strings.Join(lines[start:end], ""),
		StartLine:  start + 1,
		EndLine:    end,
		TotalLines: total,
		Truncated:  end < total,
	}, nil
}

func splitLines(content string) []string {
	if content == "" {
		return []string{}
	}

	parts := strings.SplitAfter(content, "\n")
	if parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	return parts
}
