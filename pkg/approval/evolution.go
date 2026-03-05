package approval

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func appendEvolution(workspace, title string, lines []string, now time.Time) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	date := now.UTC().Format("2006-01-02")
	path := filepath.Join(workspace, "evolution", date+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return err
	}

	var sb strings.Builder
	if info.Size() == 0 {
		sb.WriteString(fmt.Sprintf("# Evolution %s\n\n", date))
	}
	sb.WriteString(fmt.Sprintf("## %s %s\n", now.UTC().Format("15:04:05Z"), strings.TrimSpace(title)))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		sb.WriteString("- ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')

	if _, err := f.WriteString(sb.String()); err != nil {
		return err
	}
	return f.Sync()
}
