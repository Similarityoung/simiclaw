package logcapture

import (
	"bufio"
	"io"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func CaptureStdout(t testing.TB, fn func()) (out string) {
	t.Helper()

	old := os.Stdout
	oldLogger := zap.L()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	done := make(chan string, 1)
	os.Stdout = w
	defer func() {
		os.Stdout = old
		_ = w.Close()
		out = <-done
		_ = r.Close()
		zap.ReplaceGlobals(oldLogger)
	}()

	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	return out
}

func FirstNonEmptyLine(t testing.TB, out string) string {
	t.Helper()

	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return line
		}
	}
	t.Fatal("expected log output")
	return ""
}

func AssertContainsInOrder(t testing.TB, line string, parts ...string) {
	t.Helper()

	last := -1
	for _, part := range parts {
		idx := strings.Index(line, part)
		if idx < 0 {
			t.Fatalf("missing %q in %q", part, line)
		}
		if idx <= last {
			t.Fatalf("out of order %q in %q", part, line)
		}
		last = idx
	}
}
