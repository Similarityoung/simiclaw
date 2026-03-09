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
		logging.L("gateway").Info("ingest accepted", logging.String("key", "value"))
		logging.Sync()
	})

	line := firstNonEmptyLine(out)
	if line == "" {
		t.Fatal("expected log output")
	}

	parts := strings.Split(line, "\t")
	if len(parts) < 5 {
		t.Fatalf("unexpected console output: %q", line)
	}
	if matched := regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}[+-]\d{4}$`).MatchString(parts[0]); !matched {
		t.Fatalf("unexpected timestamp: %q", parts[0])
	}
	if parts[1] != "INFO" {
		t.Fatalf("level=%q", parts[1])
	}
	if !strings.Contains(parts[2], "logger_test.go") {
		t.Fatalf("caller=%q", parts[2])
	}
	if parts[3] != "ingest accepted" {
		t.Fatalf("msg=%q", parts[3])
	}
	if !strings.Contains(parts[4], `"module": "gateway"`) {
		t.Fatalf("module fields=%q", parts[4])
	}
	if !strings.Contains(parts[4], `"key": "value"`) {
		t.Fatalf("key fields=%q", parts[4])
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
