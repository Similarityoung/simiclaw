package approval

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
	store "github.com/similarityyoung/simiclaw/pkg/persistence"
)

type actionLedgerRow struct {
	ActionIdempotencyKey string                 `json:"action_idempotency_key"`
	Kind                 string                 `json:"kind"`
	RecordedAt           time.Time              `json:"recorded_at"`
	Result               model.PatchApplyResult `json:"result"`
}

type PatchExecutor struct {
	workspace   string
	ledgerPath  string
	mu          sync.Mutex
	ledgerCache map[string]model.PatchApplyResult
}

func NewPatchExecutor(workspace string) (*PatchExecutor, error) {
	exec := &PatchExecutor{
		workspace:   workspace,
		ledgerPath:  filepath.Join(workspace, "runtime", "idempotency", "action_keys.jsonl"),
		ledgerCache: map[string]model.PatchApplyResult{},
	}
	rows, err := store.ReadJSONLines[actionLedgerRow](exec.ledgerPath)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ActionIdempotencyKey == "" {
			continue
		}
		exec.ledgerCache[row.ActionIdempotencyKey] = row.Result
	}
	return exec, nil
}

func (e *PatchExecutor) Apply(payload model.PatchPayload, now time.Time) (model.PatchApplyResult, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	if payload.PatchIdempotencyKey != "" {
		if res, ok := e.lookupLedger(payload.PatchIdempotencyKey); ok {
			res.FromIdempotent = true
			return res, nil
		}
	}

	res := model.PatchApplyResult{
		OK:         false,
		TargetPath: payload.TargetPath,
	}
	if strings.TrimSpace(payload.PatchFormat) != "unified-diff" {
		res.Message = "patch_format 仅支持 unified-diff"
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}
	relPath, err := normalizePatchTargetPath(payload.TargetPath)
	if err != nil {
		res.Message = err.Error()
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}
	res.TargetPath = relPath
	absPath := filepath.Join(e.workspace, filepath.FromSlash(relPath))
	original, err := os.ReadFile(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Message = "target file not found"
			return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
		}
		return model.PatchApplyResult{}, err
	}

	currentHash := hashRawBytes(original)
	res.CurrentHash = currentHash
	res.ExpectedHash = payload.ExpectedBaseHash
	if payload.ExpectedBaseHash == "" {
		res.Message = "expected_base_hash is required"
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}
	if payload.ExpectedBaseHash != currentHash {
		res.Message = "expected_base_hash mismatch"
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}

	patched, err := applyUnifiedDiffSingle(original, payload.Diff)
	if err != nil {
		res.Message = fmt.Sprintf("apply patch failed: %v", err)
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}

	if err := store.AtomicWriteFile(absPath, patched, 0o644); err != nil {
		return model.PatchApplyResult{}, err
	}
	if err := guardPatchedContent(relPath, patched); err != nil {
		if rollbackErr := store.AtomicWriteFile(absPath, original, 0o644); rollbackErr != nil {
			return model.PatchApplyResult{}, fmt.Errorf("patch guard failed: %v; rollback failed: %v", err, rollbackErr)
		}
		res.RolledBack = true
		res.Message = fmt.Sprintf("patch guard failed and rolled back: %v", err)
		return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
	}

	res.OK = true
	res.Message = "patch applied"
	res.AppliedHash = hashRawBytes(patched)
	return e.persistIfNeeded(payload.PatchIdempotencyKey, res, now)
}

func (e *PatchExecutor) persistIfNeeded(key string, res model.PatchApplyResult, now time.Time) (model.PatchApplyResult, error) {
	if key == "" {
		return res, nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if existing, ok := e.ledgerCache[key]; ok {
		existing.FromIdempotent = true
		return existing, nil
	}
	row := actionLedgerRow{
		ActionIdempotencyKey: key,
		Kind:                 "patch",
		RecordedAt:           now,
		Result:               res,
	}
	if err := store.AppendJSONL(e.ledgerPath, row); err != nil {
		return model.PatchApplyResult{}, err
	}
	e.ledgerCache[key] = res
	return res, nil
}

func (e *PatchExecutor) lookupLedger(key string) (model.PatchApplyResult, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	res, ok := e.ledgerCache[key]
	return res, ok
}

func normalizePatchTargetPath(raw string) (string, error) {
	p := strings.TrimSpace(raw)
	if p == "" {
		return "", fmt.Errorf("target_path is required")
	}
	clean := filepath.Clean(filepath.FromSlash(p))
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("target_path must stay within workspace")
	}
	rel := filepath.ToSlash(clean)
	if !strings.HasPrefix(rel, "workflows/") && !strings.HasPrefix(rel, "skills/") {
		return "", fmt.Errorf("target_path only allows workflows/** or skills/**")
	}
	return rel, nil
}

func hashRawBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return "sha256:" + hex.EncodeToString(sum[:])
}

var hunkRE = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

func applyUnifiedDiffSingle(original []byte, diff string) ([]byte, error) {
	if strings.TrimSpace(diff) == "" {
		return nil, fmt.Errorf("diff is empty")
	}
	origLines, trailingNewline := splitLines(string(original))
	diff = strings.ReplaceAll(diff, "\r\n", "\n")
	diffLines := strings.Split(diff, "\n")

	hunkStart := -1
	for i, line := range diffLines {
		if strings.HasPrefix(line, "@@ ") {
			hunkStart = i
			break
		}
	}
	if hunkStart < 0 {
		return nil, fmt.Errorf("no hunk found")
	}

	out := make([]string, 0, len(origLines)+8)
	oldPos := 1
	i := hunkStart
	for i < len(diffLines) {
		for i < len(diffLines) && diffLines[i] == "" {
			i++
		}
		if i >= len(diffLines) {
			break
		}
		line := diffLines[i]
		if !strings.HasPrefix(line, "@@ ") {
			return nil, fmt.Errorf("unexpected line before hunk: %q", line)
		}
		m := hunkRE.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("invalid hunk header: %q", line)
		}
		oldStart, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("invalid old start: %v", err)
		}
		oldCount, err := parseHunkCount(m[2])
		if err != nil {
			return nil, fmt.Errorf("invalid old count: %v", err)
		}
		newCount, err := parseHunkCount(m[4])
		if err != nil {
			return nil, fmt.Errorf("invalid new count: %v", err)
		}
		for oldPos < oldStart {
			if oldPos > len(origLines) {
				return nil, fmt.Errorf("hunk start out of range")
			}
			out = append(out, origLines[oldPos-1])
			oldPos++
		}
		i++
		oldSeen := 0
		newSeen := 0
		for (oldSeen < oldCount || newSeen < newCount) && i < len(diffLines) {
			hunkLine := diffLines[i]
			if strings.HasPrefix(hunkLine, "@@ ") {
				return nil, fmt.Errorf("hunk truncated before reaching expected count")
			}
			if strings.HasPrefix(hunkLine, `\ No newline at end of file`) {
				i++
				continue
			}
			if hunkLine == "" {
				// Some generators may emit raw blank context lines without a leading space.
				if oldPos > len(origLines) || origLines[oldPos-1] != "" {
					return nil, fmt.Errorf("blank context mismatch at line %d", oldPos)
				}
				out = append(out, "")
				oldPos++
				oldSeen++
				newSeen++
				i++
				continue
			}
			op := hunkLine[0]
			text := hunkLine[1:]
			switch op {
			case ' ':
				if oldPos > len(origLines) || origLines[oldPos-1] != text {
					return nil, fmt.Errorf("context mismatch at line %d", oldPos)
				}
				out = append(out, text)
				oldPos++
				oldSeen++
				newSeen++
			case '-':
				if oldPos > len(origLines) || origLines[oldPos-1] != text {
					return nil, fmt.Errorf("delete mismatch at line %d", oldPos)
				}
				oldPos++
				oldSeen++
			case '+':
				out = append(out, text)
				newSeen++
			default:
				return nil, fmt.Errorf("unsupported hunk op: %q", string(op))
			}
			i++
		}
		if oldSeen != oldCount || newSeen != newCount {
			return nil, fmt.Errorf("hunk count mismatch old=%d/%d new=%d/%d", oldSeen, oldCount, newSeen, newCount)
		}
	}
	for oldPos <= len(origLines) {
		out = append(out, origLines[oldPos-1])
		oldPos++
	}
	joined := strings.Join(out, "\n")
	if trailingNewline {
		joined += "\n"
	}
	return []byte(joined), nil
}

func parseHunkCount(raw string) (int, error) {
	if raw == "" {
		return 1, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, err
	}
	return n, nil
}

func splitLines(raw string) ([]string, bool) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	trailing := strings.HasSuffix(raw, "\n")
	if trailing {
		raw = strings.TrimSuffix(raw, "\n")
	}
	if raw == "" {
		return nil, trailing
	}
	return strings.Split(raw, "\n"), trailing
}

func guardPatchedContent(relPath string, b []byte) error {
	text := string(b)
	if strings.Contains(text, "<<<<<<<") || strings.Contains(text, ">>>>>>>") {
		return fmt.Errorf("conflict marker detected")
	}
	ext := strings.ToLower(filepath.Ext(relPath))
	switch ext {
	case ".yaml", ".yml":
		lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
		hasField := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, "- ") {
				continue
			}
			if !strings.Contains(trimmed, ":") {
				return fmt.Errorf("yaml guard failed at line %d", i+1)
			}
			hasField = true
		}
		if !hasField {
			return fmt.Errorf("yaml guard failed: no key-value field")
		}
	case ".json":
		var v any
		if err := json.Unmarshal(b, &v); err != nil {
			return fmt.Errorf("json guard failed: %v", err)
		}
	}
	return nil
}
