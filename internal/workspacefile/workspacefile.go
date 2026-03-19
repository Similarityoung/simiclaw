package workspacefile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const MaxBytes = 256 * 1024

const DefaultFileMode = 0o644

const (
	CodeInvalidArgument = "invalid_argument"
	CodeNotFound        = "not_found"
	CodeConflict        = "conflict"
	CodeForbidden       = "forbidden"
)

type Error struct {
	Code    string
	Message string
	Details map[string]any
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

type PathInfo struct {
	RequestPath   string
	EffectivePath string
	AbsPath       string
	Exists        bool
	ChannelType   string
}

type pathCandidate struct {
	RequestPath   string
	WorkspaceReal string
	CandidateAbs  string
}

type PatchArgs struct {
	Path    string
	OldText string
	NewText string
	Create  bool
}

type PatchResult struct {
	Path         string `json:"path"`
	Operation    string `json:"operation"`
	BytesWritten int    `json:"bytes_written"`
	SHA256       string `json:"sha256"`
}

type DeleteArgs struct {
	Path string
}

type DeleteResult struct {
	Path      string `json:"path"`
	Operation string `json:"operation"`
}

func ResolvePath(workspace, rawPath, channelType string) (PathInfo, error) {
	candidate, err := resolveWorkspaceCandidate(workspace, rawPath)
	if err != nil {
		return PathInfo{}, err
	}

	stat, statErr := os.Lstat(candidate.CandidateAbs)
	exists := statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return PathInfo{}, statErr
	}

	effectiveAbs, effectiveRel, err := resolveEffectivePath(candidate.WorkspaceReal, candidate.CandidateAbs, exists)
	if err != nil {
		return PathInfo{}, err
	}
	if isRuntimePath(effectiveRel) {
		return PathInfo{}, &Error{Code: CodeForbidden, Message: fmt.Sprintf("path denied: %q is under runtime/", candidate.RequestPath)}
	}
	if isPrivateMemoryPath(effectiveRel) && !strings.EqualFold(strings.TrimSpace(channelType), "dm") {
		return PathInfo{}, &Error{Code: CodeForbidden, Message: "path denied: private memory requires dm channel"}
	}
	if exists && stat.IsDir() {
		return PathInfo{}, &Error{Code: CodeInvalidArgument, Message: "path points to a directory"}
	}

	return PathInfo{
		RequestPath:   candidate.RequestPath,
		EffectivePath: effectiveRel,
		AbsPath:       effectiveAbs,
		Exists:        exists,
		ChannelType:   channelType,
	}, nil
}

func Patch(workspace, channelType string, args PatchArgs) (PatchResult, error) {
	pathInfo, err := ResolvePath(workspace, args.Path, channelType)
	if err != nil {
		return PatchResult{}, err
	}

	normalizedNew := normalizeText(args.NewText)
	if err := validateTextContent([]byte(normalizedNew)); err != nil {
		return PatchResult{}, err
	}

	if !pathInfo.Exists {
		if !args.Create {
			return PatchResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		if err := os.MkdirAll(filepath.Dir(pathInfo.AbsPath), 0o755); err != nil {
			return PatchResult{}, err
		}
		if err := AtomicWriteFile(pathInfo.AbsPath, []byte(normalizedNew), DefaultFileMode); err != nil {
			return PatchResult{}, err
		}
		return buildPatchResult(pathInfo.RequestPath, "created", []byte(normalizedNew)), nil
	}

	if args.Create {
		return PatchResult{}, &Error{Code: CodeConflict, Message: fmt.Sprintf("create requested but file already exists: %s", pathInfo.RequestPath)}
	}
	if args.OldText == "" {
		return PatchResult{}, &Error{Code: CodeInvalidArgument, Message: "old_text is required when patching an existing file"}
	}
	info, err := os.Stat(pathInfo.AbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PatchResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return PatchResult{}, err
	}
	if !info.Mode().IsRegular() {
		return PatchResult{}, &Error{Code: CodeInvalidArgument, Message: "path does not point to a regular file"}
	}

	data, err := os.ReadFile(pathInfo.AbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return PatchResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return PatchResult{}, err
	}
	if err := validateTextContent(data); err != nil {
		return PatchResult{}, err
	}

	normalizedOld := normalizeText(args.OldText)
	current := normalizeText(string(data))
	matchCount := strings.Count(current, normalizedOld)
	if matchCount != 1 {
		return PatchResult{}, &Error{
			Code:    CodeConflict,
			Message: fmt.Sprintf("old_text must match exactly once in %s", pathInfo.RequestPath),
			Details: map[string]any{"match_count": matchCount},
		}
	}
	updated := strings.Replace(current, normalizedOld, normalizedNew, 1)
	if err := validateTextContent([]byte(updated)); err != nil {
		return PatchResult{}, err
	}
	if err := AtomicWriteFile(pathInfo.AbsPath, []byte(updated), info.Mode().Perm()); err != nil {
		return PatchResult{}, err
	}
	return buildPatchResult(pathInfo.RequestPath, "patched", []byte(updated)), nil
}

func Delete(workspace, channelType string, args DeleteArgs) (DeleteResult, error) {
	pathInfo, err := ResolvePath(workspace, args.Path, channelType)
	if err != nil {
		return DeleteResult{}, err
	}
	if !pathInfo.Exists {
		return DeleteResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
	}

	requestAbs := filepath.Join(workspace, filepath.FromSlash(pathInfo.RequestPath))
	requestInfo, err := os.Lstat(requestAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return DeleteResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return DeleteResult{}, err
	}
	if requestInfo.Mode()&os.ModeSymlink != 0 {
		return DeleteResult{}, &Error{Code: CodeInvalidArgument, Message: "path points to a symlink, not a regular file"}
	}

	info, err := os.Stat(pathInfo.AbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DeleteResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return DeleteResult{}, err
	}
	if !info.Mode().IsRegular() {
		return DeleteResult{}, &Error{Code: CodeInvalidArgument, Message: "path does not point to a regular file"}
	}
	data, err := os.ReadFile(pathInfo.AbsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return DeleteResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return DeleteResult{}, err
	}
	if err := validateTextContent(data); err != nil {
		return DeleteResult{}, err
	}

	if err := os.Remove(pathInfo.AbsPath); err != nil {
		if os.IsNotExist(err) {
			return DeleteResult{}, &Error{Code: CodeNotFound, Message: fmt.Sprintf("file not found: %s", pathInfo.RequestPath)}
		}
		return DeleteResult{}, err
	}
	return DeleteResult{Path: pathInfo.RequestPath, Operation: "deleted"}, nil
}

func buildPatchResult(path, operation string, content []byte) PatchResult {
	sum := sha256.Sum256(content)
	return PatchResult{
		Path:         path,
		Operation:    operation,
		BytesWritten: len(content),
		SHA256:       hex.EncodeToString(sum[:]),
	}
}

func normalizeRequestPath(rawPath string) (string, string, error) {
	p := strings.TrimSpace(rawPath)
	if p == "" {
		return "", "", &Error{Code: CodeInvalidArgument, Message: "path is required"}
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", "", &Error{Code: CodeForbidden, Message: "path denied: outside workspace"}
	}
	return filepath.ToSlash(clean), clean, nil
}

func resolveWorkspaceCandidate(workspace, rawPath string) (pathCandidate, error) {
	requestRel, cleanPath, err := normalizeRequestPath(rawPath)
	if err != nil {
		return pathCandidate{}, err
	}
	workspaceAbs, workspaceReal, err := resolveWorkspaceRoot(workspace)
	if err != nil {
		return pathCandidate{}, err
	}
	candidateAbs, err := resolveCandidatePath(workspaceAbs, cleanPath)
	if err != nil {
		return pathCandidate{}, err
	}
	return pathCandidate{
		RequestPath:   requestRel,
		WorkspaceReal: workspaceReal,
		CandidateAbs:  candidateAbs,
	}, nil
}

func resolveWorkspaceRoot(workspace string) (string, string, error) {
	workspaceAbs, err := filepath.Abs(workspace)
	if err != nil {
		return "", "", err
	}
	workspaceReal := workspaceAbs
	if resolvedWorkspace, err := filepath.EvalSymlinks(workspaceAbs); err == nil {
		workspaceReal = resolvedWorkspace
	}
	return workspaceAbs, workspaceReal, nil
}

func resolveCandidatePath(workspaceAbs, cleanPath string) (string, error) {
	candidateAbs, err := filepath.Abs(filepath.Join(workspaceAbs, cleanPath))
	if err != nil {
		return "", err
	}
	inside, err := isWithinWorkspace(workspaceAbs, candidateAbs)
	if err != nil {
		return "", err
	}
	if !inside {
		return "", &Error{Code: CodeForbidden, Message: "path denied: outside workspace"}
	}
	return candidateAbs, nil
}

func resolveEffectivePath(workspaceReal, candidateAbs string, exists bool) (string, string, error) {
	if exists {
		resolvedAbs, resolvedRel, ok, err := resolveExistingPath(workspaceReal, candidateAbs)
		if err != nil {
			return "", "", err
		}
		if !ok {
			return "", "", &Error{Code: CodeInvalidArgument, Message: "path points to a broken symlink"}
		}
		return resolvedAbs, resolvedRel, nil
	}

	ancestor, tail, err := findExistingAncestor(candidateAbs)
	if err != nil {
		return "", "", err
	}
	resolvedAncestor, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", "", err
	}
	resolvedAncestorAbs, err := filepath.Abs(resolvedAncestor)
	if err != nil {
		return "", "", err
	}
	inside, err := isWithinWorkspace(workspaceReal, resolvedAncestorAbs)
	if err != nil {
		return "", "", err
	}
	if !inside {
		return "", "", &Error{Code: CodeForbidden, Message: "path denied: symlink escapes workspace"}
	}
	effectiveAbs := filepath.Join(append([]string{resolvedAncestorAbs}, tail...)...)
	inside, err = isWithinWorkspace(workspaceReal, effectiveAbs)
	if err != nil {
		return "", "", err
	}
	if !inside {
		return "", "", &Error{Code: CodeForbidden, Message: "path denied: symlink escapes workspace"}
	}
	rel, err := filepath.Rel(workspaceReal, effectiveAbs)
	if err != nil {
		return "", "", err
	}
	return effectiveAbs, filepath.ToSlash(filepath.Clean(rel)), nil
}

func resolveExistingPath(workspaceReal, candidateAbs string) (string, string, bool, error) {
	resolved, err := filepath.EvalSymlinks(candidateAbs)
	if err != nil {
		if os.IsNotExist(err) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	resolvedAbs, err := filepath.Abs(resolved)
	if err != nil {
		return "", "", false, err
	}
	inside, err := isWithinWorkspace(workspaceReal, resolvedAbs)
	if err != nil {
		return "", "", false, err
	}
	if !inside {
		return "", "", false, &Error{Code: CodeForbidden, Message: "path denied: symlink escapes workspace"}
	}
	rel, err := filepath.Rel(workspaceReal, resolvedAbs)
	if err != nil {
		return "", "", false, err
	}
	return resolvedAbs, filepath.ToSlash(filepath.Clean(rel)), true, nil
}

func findExistingAncestor(absPath string) (string, []string, error) {
	current := absPath
	tail := make([]string, 0, 4)
	for {
		if _, err := os.Lstat(current); err == nil {
			return current, tail, nil
		} else if !os.IsNotExist(err) {
			return "", nil, err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", nil, errors.New("no existing ancestor")
		}
		tail = append([]string{filepath.Base(current)}, tail...)
		current = parent
	}
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

func isRuntimePath(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return rel == "runtime" || strings.HasPrefix(rel, "runtime/")
}

func isPrivateMemoryPath(rel string) bool {
	rel = filepath.ToSlash(filepath.Clean(rel))
	return rel == "memory/private/MEMORY.md" || strings.HasPrefix(rel, "memory/private/")
}

func validateTextContent(content []byte) error {
	if len(content) > MaxBytes {
		return &Error{Code: CodeForbidden, Message: fmt.Sprintf("file content exceeds %d bytes", MaxBytes)}
	}
	if strings.ContainsRune(string(content), '\x00') {
		return &Error{Code: CodeForbidden, Message: "file content contains NUL bytes"}
	}
	if !utf8.Valid(content) {
		return &Error{Code: CodeForbidden, Message: "file content is not valid UTF-8 text"}
	}
	return nil
}

func normalizeText(in string) string {
	in = strings.ReplaceAll(in, "\r\n", "\n")
	in = strings.ReplaceAll(in, "\r", "\n")
	return in
}

func AtomicWriteFile(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() {
		_ = os.Remove(tmpPath)
	}()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}
