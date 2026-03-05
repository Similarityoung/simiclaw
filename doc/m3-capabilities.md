# M3 当前能力说明

本文描述仓库在 `M3` 阶段已经实现并可用的能力边界，作为「当前可交付行为」说明。

## 1. 阶段定位

- 当前阶段：`M3`（由根目录 `VERSION_STAGE` 驱动）
- 目标能力：在 M2 查询/恢复能力基础上，补齐记忆写入、RAG 召回与 NO_REPLY 内务回合闭环
- 设计取舍：保持单机单进程与 deterministic runner，不引入外部向量库/索引服务

## 2. 已实现能力

### 2.1 Memory/RAG 工具

- `memory_search`
  - 输入：`query/scope/top_k`
  - `scope=auto` 语义：
    - `dm` -> `private+public`
    - `group/channel` -> `public`
  - 输出：`path/scope/lines/score/preview`
- `memory_get`
  - 输入：`path + lines`
  - 安全约束：
    - 路径仅允许 `memory/**/*.md` 与 `MEMORY.md`
    - 拒绝绝对路径与 `..` 越界
    - 行范围合法性校验
    - 返回内容字符数上限（默认 8000）

### 2.2 记忆写入

- daily memory：追加写入 `workspace/memory/YYYY-MM-DD.md`
- curated memory：追加写入 `workspace/MEMORY.md`
- `WriteMemory` 动作可在普通回合和 NO_REPLY 内务回合执行

### 2.3 NO_REPLY 与 compaction

- `payload.type=memory_flush|compaction|cron_fire` 自动进入 `run_mode=NO_REPLY`
- NO_REPLY 回合抑制用户可见出站（`delivery_status=suppressed`）
- `compaction` 回合会写入会话内 `compaction_summary` entry，并保留 `cutoff_commit_id` 元信息

### 2.4 Run Trace 增强

`GET /v1/runs/{run_id}/trace` 返回新增字段：

- `context_manifest.history_range`
- `rag_hits`
- `tool_executions`（含 `args_summary`）

`redact=true` 时：

- 原始 `args` 脱敏
- `content/native/native_ref` 脱敏
- `args_summary` 保留用于排障

## 3. 验收入口

关键命令：

- `make test-unit`
- `make test-integration`
- `make test-e2e-smoke`
- `make accept-m3`
- `make accept-current`（当 `VERSION_STAGE=M3`）

## 4. 非 M3 范围（后续阶段）

以下能力不属于 M3 完整交付范围：

- BM25/向量混合检索、独立 indexer watcher（P1）
- approval/patch/rollback 闭环（M4）
- fault injection 常态化（M5，可选）
