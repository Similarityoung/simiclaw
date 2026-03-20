package gateway

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	telegramchannel "github.com/similarityyoung/simiclaw/internal/channels/telegram"
	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
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

	var out string
	out = logcapture.CaptureStdout(t, func() {
		err = run(Options{Workspace: filepath.Join(dir, "workspace")})
	})
	if err == nil {
		t.Fatal("expected config error")
	}
	if !strings.Contains(err.Error(), "provider/model format") {
		t.Fatalf("expected provider/model format error, got %v", err)
	}
	if strings.Contains(out, "[cmd] bootstrap failed") {
		t.Fatalf("unexpected duplicate cmd log: %q", out)
	}
}

func TestRunLogsServeFailure(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

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

	var out string
	out = logcapture.CaptureStdout(t, func() {
		err = run(Options{Workspace: workspace, Listen: ln.Addr().String()})
	})
	if err == nil {
		t.Fatal("expected serve failure")
	}
	if !strings.Contains(out, "[cmd] simiclaw serve failed") {
		t.Fatalf("expected serve failure log, got %q", out)
	}
}

func TestRunDoesNotDuplicateStartupFailureLog(t *testing.T) {
	dir := t.TempDir()
	workspace := filepath.Join(dir, "workspace")
	if err := store.InitWorkspace(workspace, false, store.DefaultBusyTimeout()); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "telegram unavailable", http.StatusBadGateway)
	}))
	defer server.Close()
	restore := telegramchannel.SetRuntimeTestHooksForTesting(server.URL, server.Client())
	defer restore()

	cfg := config.Default()
	cfg.Workspace = workspace
	cfg.Channels.Telegram.Enabled = true
	cfg.Channels.Telegram.Token = "test-token"

	body, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	cfgPath := filepath.Join(dir, "config.json")
	if err := os.WriteFile(cfgPath, body, 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out string
	out = logcapture.CaptureStdout(t, func() {
		err = run(Options{ConfigPath: cfgPath})
	})
	if err == nil {
		t.Fatal("expected startup failure")
	}
	if !strings.Contains(out, "[bootstrap] telegram runtime start failed") {
		t.Fatalf("expected bootstrap startup failure log, got %q", out)
	}
	if strings.Contains(out, "[cmd] simiclaw serve failed") {
		t.Fatalf("expected cmd layer to skip duplicate startup error log, got %q", out)
	}
}
