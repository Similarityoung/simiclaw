package initcmd

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/similarityyoung/simiclaw/internal/store"
)

func scaffoldWorkspaceFiles(workspace string) error {
	for _, rel := range sortedWorkspaceTemplateNames() {
		if err := writeTemplateIfMissing(filepath.Join(workspace, rel), workspaceTemplates[rel]); err != nil {
			return err
		}
	}
	return nil
}

func writeTemplateIfMissing(path, content string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return store.AtomicWriteFile(path, []byte(content), 0o644)
}
