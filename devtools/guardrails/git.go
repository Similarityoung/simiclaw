package guardrails

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

func listTrackedGoFiles(ctx context.Context, root string) ([]string, error) {
	stdout, err := gitOutput(ctx, root, "ls-files", "*.go")
	if err != nil {
		return nil, err
	}
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = filepath.ToSlash(line)
		if !strings.HasPrefix(line, "cmd/") && !strings.HasPrefix(line, "internal/") && !strings.HasPrefix(line, "pkg/") {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, nil
}

func listChangedGoFiles(ctx context.Context, root, base, head string) ([]string, map[string]changedFile, error) {
	nameOnly, err := gitOutput(ctx, root, "diff", "--name-only", "--diff-filter=ACMR", base, head, "--", "*.go")
	if err != nil {
		return nil, nil, err
	}
	diffText, err := gitOutput(ctx, root, "diff", "--unified=0", "--diff-filter=ACMR", base, head, "--", "*.go")
	if err != nil {
		return nil, nil, err
	}
	changes, err := parseDiff(diffText)
	if err != nil {
		return nil, nil, err
	}

	var files []string
	for _, line := range strings.Split(strings.TrimSpace(nameOnly), "\n") {
		line = filepath.ToSlash(strings.TrimSpace(line))
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "cmd/") && !strings.HasPrefix(line, "internal/") && !strings.HasPrefix(line, "pkg/") {
			continue
		}
		files = append(files, line)
	}
	sort.Strings(files)
	return files, changes, nil
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
