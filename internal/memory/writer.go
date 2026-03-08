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

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func NewWriter(workspace string) *Writer {
	return &Writer{workspace: workspace}
}

func VisibilityForChannel(channelType string) string {
	if strings.EqualFold(strings.TrimSpace(channelType), "dm") {
		return VisibilityPrivate
	}
	return VisibilityPublic
}

func NormalizeVisibility(visibility string) string {
	if strings.EqualFold(strings.TrimSpace(visibility), VisibilityPrivate) {
		return VisibilityPrivate
	}
	return VisibilityPublic
}

func DailyPath(visibility string, now time.Time) string {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return filepath.ToSlash(filepath.Join("memory", NormalizeVisibility(visibility), "daily", now.Format("2006-01-02")+".md"))
}

func CuratedPath(visibility string) string {
	return filepath.ToSlash(filepath.Join("memory", NormalizeVisibility(visibility), "MEMORY.md"))
}

func (w *Writer) WriteDaily(source, text string, now time.Time, visibility string) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rel := DailyPath(visibility, now)
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

func (w *Writer) WriteCurated(text string, now time.Time, visibility string) (string, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	rel := CuratedPath(visibility)
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
