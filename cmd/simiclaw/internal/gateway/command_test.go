package gateway

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReturnsConfigErrorWithoutConfigFile(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENAI_BASE_URL", "")
	t.Setenv("LLM_API_KEY", "")
	t.Setenv("LLM_BASE_URL", "")
	t.Setenv("LLM_MODEL", "")
	t.Setenv("SIMICLAW_LLM_DEFAULT_MODEL", "")

	dir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Fatalf("restore wd: %v", err)
		}
	}()

	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("LLM_MODEL=deepseek-chat\n"), 0o644); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	err = run(Options{Workspace: filepath.Join(dir, "workspace")})
	if err == nil {
		t.Fatal("expected config error")
	}
	if !strings.Contains(err.Error(), "provider/model format") {
		t.Fatalf("expected provider/model format error, got %v", err)
	}
}
