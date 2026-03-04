package logging_test

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
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

func TestInitAndJSONOutput(t *testing.T) {
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

	var got map[string]any
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("invalid json output: %v", err)
	}
	if got["level"] != "info" {
		t.Fatalf("level=%v", got["level"])
	}
	if got["msg"] != "ingest accepted" {
		t.Fatalf("msg=%v", got["msg"])
	}
	if got["module"] != "gateway" {
		t.Fatalf("module=%v", got["module"])
	}
	if got["key"] != "value" {
		t.Fatalf("key=%v", got["key"])
	}
	caller, ok := got["caller"].(string)
	if !ok || !strings.Contains(caller, "logger_test.go") {
		t.Fatalf("caller=%v", got["caller"])
	}
	if _, ok := got["ts"]; !ok {
		t.Fatal("missing ts field")
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

	done := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(r)
		done <- string(b)
	}()

	fn()
	_ = w.Close()
	os.Stdout = old
	_ = r.Close()

	return <-done
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
