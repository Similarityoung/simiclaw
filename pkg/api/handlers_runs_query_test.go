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
