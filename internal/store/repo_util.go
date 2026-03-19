package store

import (
	"strings"
	"time"
)

func timeText(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func mustParseTime(raw string) time.Time {
	if strings.TrimSpace(raw) == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}
	}
	return t.UTC()
}
