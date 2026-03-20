package logging

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"go.uber.org/multierr"
	"go.uber.org/zap"
)

type staticSyncErrorSink struct {
	syncErr error
}

type bytesSink struct {
	buf bytes.Buffer
}

type nonComparableError struct {
	items []int
	msg   string
}

func (e nonComparableError) Error() string {
	return e.msg
}

func (s staticSyncErrorSink) Write(p []byte) (int, error) {
	return len(p), nil
}

func (s staticSyncErrorSink) Sync() error {
	return s.syncErr
}

func (s *bytesSink) Write(p []byte) (int, error) {
	return s.buf.Write(p)
}

func (s *bytesSink) Sync() error {
	return nil
}

func TestSyncReturnsErrorWhenNonIgnorable(t *testing.T) {
	logger, err := newLogger("info", staticSyncErrorSink{syncErr: errors.New("disk full")})
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	syncErr := Sync()
	if syncErr == nil {
		t.Fatal("expected sync error")
	}
	if !strings.Contains(syncErr.Error(), "disk full") {
		t.Fatalf("unexpected sync error: %v", syncErr)
	}
}

func TestSyncIgnoresKnownStdStreamErrors(t *testing.T) {
	logger, err := newLogger("info", staticSyncErrorSink{syncErr: errors.New("bad file descriptor")})
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	if syncErr := Sync(); syncErr != nil {
		t.Fatalf("expected nil sync error, got %v", syncErr)
	}
}

func TestIsIgnorableSyncErrorHandlesMultierrAllIgnorable(t *testing.T) {
	err := multierr.Combine(errors.New("bad file descriptor"), errors.New("invalid argument"))
	if !isIgnorableSyncError(err) {
		t.Fatalf("expected ignorable multierr: %v", err)
	}
}

func TestIsIgnorableSyncErrorHandlesMultierrMixed(t *testing.T) {
	err := multierr.Combine(errors.New("bad file descriptor"), errors.New("disk full"))
	if isIgnorableSyncError(err) {
		t.Fatalf("expected non-ignorable multierr: %v", err)
	}
}

func TestIsIgnorableSyncErrorHandlesNonComparableError(t *testing.T) {
	err := nonComparableError{
		items: []int{1, 2, 3},
		msg:   "bad file descriptor",
	}
	if !isIgnorableSyncError(err) {
		t.Fatalf("expected ignorable non-comparable error: %v", err)
	}
}

func TestConsoleEncoderRendersStructuredContext(t *testing.T) {
	sink := &bytesSink{}
	logger, err := newLogger("info", sink)
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	logTime := time.Date(2026, 3, 20, 9, 30, 0, 0, time.UTC)
	L("runner").With(
		String("model", "gpt-5.4"),
		String("session_key", "cli:conv:1"),
	).Info(
		"provider failed",
		String("event_id", "evt_123"),
		String("run_id", "run_456"),
		String("empty", ""),
		Error(errors.New("disk full")),
		String("note", "quote me please"),
		Any("next_attempt_at", logTime),
	)

	line := strings.TrimSpace(sink.buf.String())
	assertContainsAll(t, line,
		[]string{
			"[runner] provider failed",
			`"event_id": "evt_123"`,
			`"run_id": "run_456"`,
			`"session_key": "cli:conv:1"`,
			`"model": "gpt-5.4"`,
			`"empty": ""`,
			`"error": "disk full"`,
			`"next_attempt_at": "2026-03-20T09:30:00.000Z"`,
			`"note": "quote me please"`,
		},
	)
}

func TestConsoleEncoderRendersStructuredData(t *testing.T) {
	sink := &bytesSink{}
	logger, err := newLogger("info", sink)
	if err != nil {
		t.Fatalf("newLogger: %v", err)
	}
	restore := zap.ReplaceGlobals(logger)
	defer restore()

	L("runner").Info("payload", Any("data", map[string]any{"a": 1, "b": 2}))

	line := strings.TrimSpace(sink.buf.String())
	if !strings.Contains(line, `"data": {"a":1,"b":2}`) {
		t.Fatalf("unexpected rendered structured value: %s", line)
	}
}

func TestShouldColorizeConsoleFor(t *testing.T) {
	cases := []struct {
		name   string
		inputs colorizeConsoleInputs
		want   bool
	}{
		{
			name: "force color overrides no tty",
			inputs: colorizeConsoleInputs{
				forceColor: true,
			},
			want: true,
		},
		{
			name: "no color disables tty",
			inputs: colorizeConsoleInputs{
				noColor:   true,
				stdoutTTY: true,
			},
			want: false,
		},
		{
			name: "tty enables color",
			inputs: colorizeConsoleInputs{
				stderrTTY: true,
			},
			want: true,
		},
		{
			name: "jetbrains run binary enables color outside tests",
			inputs: colorizeConsoleInputs{
				jetbrainsRunBinary: true,
			},
			want: true,
		},
		{
			name: "tests keep captured output uncolored",
			inputs: colorizeConsoleInputs{
				jetbrainsRunBinary: true,
				runningTests:       true,
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := shouldColorizeConsoleFor(tc.inputs); got != tc.want {
				t.Fatalf("shouldColorizeConsoleFor(%+v)=%v want %v", tc.inputs, got, tc.want)
			}
		})
	}
}

func TestLooksLikeJetBrainsRunBinary(t *testing.T) {
	cases := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "goland temporary binary",
			path: "/Users/similarityyoung/Library/Caches/JetBrains/GoLand2025.2/tmp/GoLand/___simi_server",
			want: true,
		},
		{
			name: "intellij temporary binary",
			path: "/Users/similarityyoung/Library/Caches/JetBrains/IntelliJIdea2025.2/tmp/IntelliJIdea/___app",
			want: true,
		},
		{
			name: "normal binary path",
			path: "/Users/similarityyoung/Documents/SimiClaw/bin/simiclaw",
			want: false,
		},
		{
			name: "empty path",
			path: "",
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := looksLikeJetBrainsRunBinary(tc.path); got != tc.want {
				t.Fatalf("looksLikeJetBrainsRunBinary(%q)=%v want %v", tc.path, got, tc.want)
			}
		})
	}
}

func assertContainsAll(t *testing.T, line string, parts []string) {
	t.Helper()

	for _, part := range parts {
		if !strings.Contains(line, part) {
			t.Fatalf("missing %q in %q", part, line)
		}
	}
}
