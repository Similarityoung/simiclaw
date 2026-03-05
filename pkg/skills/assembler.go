package skills

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const SkillsInjectionHeader = "Workspace skill instructions (auto-injected from workspace/skills/**/SKILL.md):"

func AssembleInstructionInjection(workspace string) (string, error) {
	workspace = strings.TrimSpace(workspace)
	if workspace == "" {
		return "", errors.New("invalid_argument: workspace is required")
	}

	skillsRoot := filepath.Join(workspace, "skills")
	info, err := os.Stat(skillsRoot)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("stat skills directory: %w", err)
	}
	if !info.IsDir() {
		return "", nil
	}

	type skillFile struct {
		relPath string
		absPath string
	}

	files := make([]skillFile, 0)
	if err := filepath.WalkDir(skillsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		rel, err := filepath.Rel(skillsRoot, path)
		if err != nil {
			return err
		}
		files = append(files, skillFile{relPath: filepath.ToSlash(rel), absPath: path})
		return nil
	}); err != nil {
		return "", fmt.Errorf("scan skills directory: %w", err)
	}

	if len(files) == 0 {
		return "", nil
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].relPath < files[j].relPath
	})

	var b strings.Builder
	b.WriteString(SkillsInjectionHeader)
	for _, f := range files {
		content, err := os.ReadFile(f.absPath)
		if err != nil {
			return "", fmt.Errorf("read skill file %q: %w", f.relPath, err)
		}
		b.WriteString("\n\n")
		b.WriteString("### ")
		b.WriteString(f.relPath)
		b.WriteString("\n")
		b.Write(content)
	}

	return b.String(), nil
}
