package bootstrap

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/similarityyoung/simiclaw/internal/config"
	"github.com/similarityyoung/simiclaw/internal/store"
	"github.com/similarityyoung/simiclaw/internal/testutil/logcapture"
	"github.com/similarityyoung/simiclaw/pkg/logging"
)

func TestNewAppLogsProviderFactoryFailure(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	cfg.LLM.DefaultModel = "bad-model"
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	var appErr error
	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		_, appErr = NewApp(cfg)
		_ = logging.Sync()
	})

	if appErr == nil {
		t.Fatal("expected NewApp to fail")
	}
	if !strings.Contains(out, "[bootstrap] provider factory failed") {
		t.Fatalf("expected provider factory failure log, got %q", out)
	}
}

func TestAppStartStopLogsLifecycle(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		app, err := NewApp(cfg)
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		if err := app.Start(context.Background()); err != nil {
			t.Fatalf("Start: %v", err)
		}
		app.Stop()
		_ = logging.Sync()
	})

	logcapture.AssertContainsInOrder(t, out,
		"[bootstrap] database opened",
		"[bootstrap] application assembled",
		"[bootstrap] runtime supervisor started",
		"[bootstrap] application stopping",
		"[bootstrap] application stopped",
	)
}

func TestAppStartLogsSupervisorStartFailure(t *testing.T) {
	cfg := config.Default()
	cfg.Workspace = t.TempDir()
	if err := store.InitWorkspace(cfg.Workspace, false, cfg.DBBusyTimeout.Duration); err != nil {
		t.Fatalf("InitWorkspace: %v", err)
	}

	out := logcapture.CaptureStdout(t, func() {
		if err := logging.Init("info"); err != nil {
			t.Fatalf("Init error: %v", err)
		}
		app, err := NewApp(cfg)
		if err != nil {
			t.Fatalf("NewApp: %v", err)
		}
		if err := app.Start(nil); err == nil {
			t.Fatal("expected Start to fail")
		} else if !errors.Is(err, ErrStartup) {
			t.Fatalf("expected ErrStartup, got %v", err)
		}
		_ = logging.Sync()
	})

	if !strings.Contains(out, "[bootstrap] runtime supervisor start failed") {
		t.Fatalf("expected supervisor start failure log, got %q", out)
	}
}
