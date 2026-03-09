package workspace

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/similarityyoung/simiclaw/internal/store"
)

func ScaffoldFiles(root string) error {
	for _, rel := range templateNames() {
		if err := writeTemplateIfMissing(filepath.Join(root, rel), templates[rel]); err != nil {
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
