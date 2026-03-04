package store

import (
	"os"
	"path/filepath"
	"time"

	"github.com/similarityyoung/simiclaw/pkg/model"
)

// InitWorkspace 创建运行所需目录与初始化文件，确保工作区可用。
func InitWorkspace(workspace string) error {
	// 预创建运行时目录，避免后续各模块首次写入时出现路径不存在。
	paths := []string{
		filepath.Join(workspace, "runtime"),
		filepath.Join(workspace, "runtime", "sessions"),
		filepath.Join(workspace, "runtime", "runs"),
		filepath.Join(workspace, "runtime", "idempotency"),
		filepath.Join(workspace, "runtime", "outbound_spool"),
		filepath.Join(workspace, "runtime", "native"),
		filepath.Join(workspace, "runtime", "events"),
		filepath.Join(workspace, "tests"),
	}
	for _, p := range paths {
		if err := os.MkdirAll(p, 0o755); err != nil {
			return err
		}
	}

	// 初始化会话索引文件，记录 session_key -> active_session_id 的映射。
	sessionsPath := filepath.Join(workspace, "runtime", "sessions.json")
	if _, err := os.Stat(sessionsPath); os.IsNotExist(err) {
		idx := model.SessionIndex{
			FormatVersion: "1",
			UpdatedAt:     time.Now().UTC(),
			Sessions:      map[string]model.SessionIndexRow{},
		}
		if err := AtomicWriteJSON(sessionsPath, idx, 0o644); err != nil {
			return err
		}
	}

	// 预置 cron 作业文件，保证计划任务模块可直接读写。
	jobsPath := filepath.Join(workspace, "runtime", "cron", "jobs.json")
	if err := os.MkdirAll(filepath.Dir(jobsPath), 0o755); err != nil {
		return err
	}
	if _, err := os.Stat(jobsPath); os.IsNotExist(err) {
		if err := AtomicWriteFile(jobsPath, []byte("{\n  \"jobs\": []\n}\n"), 0o644); err != nil {
			return err
		}
	}

	// 预置事件仓库文件，避免 EventRepo 首次加载失败。
	eventsPath := filepath.Join(workspace, "runtime", "events", "events.json")
	if _, err := os.Stat(eventsPath); os.IsNotExist(err) {
		if err := AtomicWriteFile(eventsPath, []byte("{\n  \"events\": {}\n}\n"), 0o644); err != nil {
			return err
		}
	}

	// 预创建幂等账本文件，供 inbound/outbound/action 去重逻辑使用。
	for _, f := range []string{"inbound_keys.jsonl", "outbound_keys.jsonl", "action_keys.jsonl"} {
		p := filepath.Join(workspace, "runtime", "idempotency", f)
		if _, err := os.Stat(p); os.IsNotExist(err) {
			if err := AtomicWriteFile(p, []byte(""), 0o644); err != nil {
				return err
			}
		}
	}

	return nil
}
