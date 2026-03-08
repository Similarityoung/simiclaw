package inspect

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

func TestRenderJSON(t *testing.T) {
	var buf bytes.Buffer
	payload := map[string]any{"status": "ok"}
	if err := render(&buf, "json", payload, func(_ io.Writer) {
		t.Fatal("table renderer should not be called for json output")
	}); err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if !strings.Contains(buf.String(), `"status": "ok"`) {
		t.Fatalf("expected json output, got %q", buf.String())
	}
}

func TestRenderTable(t *testing.T) {
	var buf bytes.Buffer
	called := false
	if err := render(&buf, "table", map[string]any{"status": "ok"}, func(w io.Writer) {
		called = true
		_, _ = io.WriteString(w, "status\tok\n")
	}); err != nil {
		t.Fatalf("render() error = %v", err)
	}
	if !called {
		t.Fatal("expected table renderer to be called")
	}
	if got := buf.String(); got != "status\tok\n" {
		t.Fatalf("unexpected table output %q", got)
	}
}
