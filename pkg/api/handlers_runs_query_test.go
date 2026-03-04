package api

import "testing"

func TestRedactMapKeepsArgsKey(t *testing.T) {
	in := map[string]any{
		"args": map[string]any{
			"query": "secret",
		},
	}

	out := redactMap(in)
	if _, ok := out["args_summary"]; ok {
		t.Fatalf("args_summary should not be present in redacted output")
	}
	v, ok := out["args"]
	if !ok {
		t.Fatalf("args key should be preserved")
	}
	if v != "<redacted>" {
		t.Fatalf("args should be redacted in place, got=%v", v)
	}
}

func TestRedactMapKeepsArgsSummary(t *testing.T) {
	in := map[string]any{
		"args": map[string]any{
			"path": "../../go.mod",
		},
		"args_summary": map[string]any{
			"keys": []string{"path"},
		},
	}

	out := redactMap(in)
	if out["args"] != "<redacted>" {
		t.Fatalf("args should be redacted, got=%v", out["args"])
	}
	if _, ok := out["args_summary"]; !ok {
		t.Fatalf("args_summary should remain in redacted output")
	}
}
