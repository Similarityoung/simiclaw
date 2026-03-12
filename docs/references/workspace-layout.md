# Workspace Layout Reference

## Summary

workspace 既是运行时状态目录，也是 prompt / memory / skills 的输入面。SimiClaw 对哪些路径允许存在、哪些文件会被 scaffold、哪些路径允许被工具读写，都有明确限制。

## 运行时允许的目录

初始化后的 workspace 只应该包含这些运行时路径：

```text
workspace/
  memory/
  runtime/
    app.db
    native/
```

`store.InitWorkspace` 会创建 `runtime/native`、`memory` 和 SQLite 数据库；旧文件式 runtime 痕迹会被拒绝，除非显式使用 `--force-new-runtime` 进行清理迁移。

## Legacy 拒绝列表

当前代码会把下列旧路径视为 legacy runtime 痕迹：

- `runtime/sessions.json`
- `runtime/sessions`
- `runtime/runs`
- `runtime/approvals`
- `runtime/idempotency`
- `runtime/outbound_spool`
- `evolution`

## Scaffold 的上下文文件

`init` 会在缺失时创建这些模板文件，但不会覆盖已有内容：

- `SOUL.md`
- `IDENTITY.md`
- `USER.md`
- `TOOLS.md`
- `BOOTSTRAP.md`
- `HEARTBEAT.md`

`AGENTS.md` 需要由项目自行维护，不会自动生成。

## Prompt 注入面

会被 prompt builder 自动读取的主要输入包括：

- workspace 根部上下文文件
- `memory/public/MEMORY.md`
- `memory/private/MEMORY.md`（仅 `dm`）
- `workspace/skills/*/SKILL.md` 的摘要索引

不会自动注入但可以被工具访问的内容包括：

- skill 正文（通过 `context_get`）
- daily memory（通过 `memory_search` / `memory_get`）

## 文件读写边界

- `context_get` 只能读取固定上下文文件和 skill 正文
- `workspace_patch` / `workspace_delete` 只能处理 workspace 内的 UTF-8 文本文件
- `runtime/` 下的文件对 workspace 写工具是禁止路径
- 非 `dm` 会话不能碰 private memory 路径

## Verification

- workspace 初始化: `cmd/simiclaw/internal/initcmd/command.go`
- runtime 目录与 legacy 检测: `internal/store/db.go`
- scaffold 模板: `internal/workspace/scaffold.go`, `internal/workspace/templates/`
- prompt 输入: `internal/prompt/loader.go`
- 文件安全边界: `internal/workspacefile/workspacefile.go`

## Related Docs

- Prompt / Workspace 设计: [`../design-docs/prompt-and-workspace-context.md`](../design-docs/prompt-and-workspace-context.md)
- 配置参考: [`configuration.md`](configuration.md)
- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
