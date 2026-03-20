package logging_test

import (
	"bufio"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func TestParseLevel(t *testing.T) {
	cases := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "debug", want: "debug"},
		{in: "info", want: "info"},
		{in: "warn", want: "warn"},
		{in: "error", want: "error"},
		{in: "INFO", want: "info"},
		{in: "bad", wantErr: true},
	}

	for _, tc := range cases {
		got, err := logging.ParseLevel(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("ParseLevel(%q) expected error", tc.in)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseLevel(%q) error: %v", tc.in, err)
		}
		if got.String() != tc.want {
			t.Fatalf("ParseLevel(%q)=%s want=%s", tc.in, got.String(), tc.want)
		}
	}
}

func TestInitAndConsoleOutput(t *testing.T) {
	out := captureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		logging.L("gateway").Info(
			"ingest accepted",
			logging.String("model", "gpt-5.4"),
			logging.String("session_key", "cli:conv:1"),
			logging.String("event_id", "evt_123"),
			logging.String("run_id", "run_456"),
			logging.String("detail", "hello world"),
		)
		logging.Sync()
	})

	line := firstNonEmptyLine(out)
	if matched := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4} INFO .*logger_test\.go:\d+ \[gateway\] ingest accepted `).MatchString(line); !matched {
		t.Fatalf("unexpected line=%q", line)
	}
	assertFieldSequence(t, line, []string{
		"event_id=evt_123",
		"run_id=run_456",
		"session_key=cli:conv:1",
		"model=gpt-5.4",
		`detail="hello world"`,
	})
	if strings.Contains(line, `"module"`) {
		t.Fatalf("unexpected module field=%q", line)
	}
}

func TestLoggerWithoutModuleDoesNotPrefixMessage(t *testing.T) {
	out := captureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		logging.L("").Info("plain message")
		logging.Sync()
	})

	line := firstNonEmptyLine(out)
	if !strings.Contains(line, " plain message") {
		t.Fatalf("msg=%q", line)
	}
}

func TestCorrelationFieldsFollowCanonicalOrder(t *testing.T) {
	out := captureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		logging.L("runtime.kernel").Info(
			"completed",
			logging.String("model", "gpt-5.4"),
			logging.String("tool_name", "web_search"),
			logging.String("event_id", "evt_123"),
			logging.String("provider", "openai"),
			logging.String("session_key", "cli:conv:1"),
			logging.String("run_id", "run_456"),
			logging.String("payload_type", "message"),
			logging.String("outbox_id", "out_789"),
			logging.String("tool_call_id", "call_321"),
			logging.String("worker", "delivery_poll"),
			logging.String("session_id", "ses_999"),
			logging.String("job_id", "job_654"),
		)
		logging.Sync()
	})

	line := firstNonEmptyLine(out)
	assertFieldSequence(t, line, []string{
		"event_id=evt_123",
		"run_id=run_456",
		"session_key=cli:conv:1",
		"session_id=ses_999",
		"payload_type=message",
		"outbox_id=out_789",
		"job_id=job_654",
		"worker=delivery_poll",
		"tool_call_id=call_321",
		"tool_name=web_search",
		"provider=openai",
		"model=gpt-5.4",
	})
}

func TestNilLoggerWithDoesNotPanic(t *testing.T) {
	out := captureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		var logger *logging.Logger
		logger.With(logging.String("key", "value")).Info("plain message")
		logging.Sync()
	})

	line := firstNonEmptyLine(out)
	if !strings.Contains(line, " plain message") {
		t.Fatalf("msg=%q", line)
	}
	if !strings.Contains(line, " key=value") {
		t.Fatalf("key fields=%q", line)
	}
}

func TestInitRejectsInvalidLevel(t *testing.T) {
	if err := logging.Init("not-valid"); err == nil {
		t.Fatal("expected Init to fail")
	}
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("create pipe: %v", err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = old
	}()

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	_ = w.Close()
	out := <-done
	_ = r.Close()

	return out
}

func assertFieldSequence(t *testing.T, line string, fields []string) {
	t.Helper()

	lastIndex := -1
	for _, field := range fields {
		idx := strings.Index(line, field)
		if idx < 0 {
			t.Fatalf("missing field %q in %q", field, line)
		}
		if idx <= lastIndex {
			t.Fatalf("field %q out of order in %q", field, line)
		}
		lastIndex = idx
	}
}

func firstNonEmptyLine(out string) string {
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			return line
		}
	}
	return ""
}
