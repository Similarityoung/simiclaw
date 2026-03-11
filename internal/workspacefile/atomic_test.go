package workspacefile

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestAtomicWriteFileWritesJSONDocument(t *testing.T) {
	jsonPath := filepath.Join(t.TempDir(), "config.json")
	body, err := json.MarshalIndent(map[string]string{"status": "ok"}, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent: %v", err)
	}
	body = append(body, '\n')
	if err := AtomicWriteFile(jsonPath, body, 0o644); err != nil {
		t.Fatalf("AtomicWriteFile: %v", err)
	}
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var decoded map[string]string
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}
	if decoded["status"] != "ok" {
		t.Fatalf("unexpected decoded content: %+v", decoded)
	}
}

func TestAtomicWriteFileFailsWhenDirectoryMissing(t *testing.T) {
	root := t.TempDir()
	if err := AtomicWriteFile(filepath.Join(root, "missing", "config.json"), []byte("{}\n"), 0o644); err == nil {
		t.Fatalf("expected AtomicWriteFile to fail for missing directory")
	}
}
