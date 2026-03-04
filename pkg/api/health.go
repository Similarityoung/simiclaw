package api

import (
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// handleHealthz 返回进程存活状态。
func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// handleReadyz 检查运行目录和关键文件，返回服务就绪状态。
func (a *App) handleReadyz(w http.ResponseWriter, _ *http.Request) {
	paths := []string{
		filepath.Join(a.Cfg.Workspace, "runtime", "sessions.json"),
		filepath.Join(a.Cfg.Workspace, "runtime", "sessions"),
		filepath.Join(a.Cfg.Workspace, "runtime", "runs"),
	}
	for _, p := range paths {
		if _, err := os.Stat(p); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "not_ready", "error": err.Error()})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":      "ready",
		"queue_depth": a.Bus.InboundDepth(),
		"time":        time.Now().UTC().Format(time.RFC3339Nano),
	})
}
