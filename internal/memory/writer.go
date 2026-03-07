package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type Writer struct {
	workspace string
}

func NewWriter(workspace string) *Writer {
	return &Writer{workspace: workspace}
}

func (w *Writer) WriteDaily(source, text string, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rel := filepath.ToSlash(filepath.Join("memory", now.Format("2006-01-02")+".md"))
	abs := filepath.Join(w.workspace, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	line := fmt.Sprintf("- [%s] (%s) %s\n", now.UTC().Format(time.RFC3339), sanitize(source), sanitize(text))
	f, err := os.OpenFile(abs, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return rel, nil
}

func (w *Writer) WriteCurated(text string, now time.Time) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rel := "MEMORY.md"
	abs := filepath.Join(w.workspace, rel)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return "", err
	}
	if _, err := os.Stat(abs); os.IsNotExist(err) {
		init := "# Curated Memory\n\n"
		if err := os.WriteFile(abs, []byte(init), 0o644); err != nil {
			return "", err
		}
	}
	line := fmt.Sprintf("- %s %s\n", now.UTC().Format("2006-01-02"), sanitize(text))
	f, err := os.OpenFile(abs, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return "", err
	}
	if err := f.Sync(); err != nil {
		return "", err
	}
	return rel, nil
}

func sanitize(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\n", " "))
	if s == "" {
		return "-"
	}
	return s
}
