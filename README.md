# SimiClaw (v1.0 alpha)

单机单进程 Go Agent Runtime，当前阶段为 `V1_ALPHA`。运行时采用 `SQLite-first` 架构，数据库固定为 `workspace/runtime/app.db`，`sessions` 仅作为派生缓存，SQLite 是唯一事实源。

## 当前能力

- CLI：`chat | inspect | init | serve | version | completion`
- HTTP：
  - `POST /v1/chat:stream`
  - `POST /v1/events:ingest`
  - `GET /v1/events/{event_id}`
  - `GET /v1/events`
  - `GET /v1/events:lookup`
  - `GET /v1/runs`
  - `GET /v1/runs/{run_id}`
  - `GET /v1/runs/{run_id}/trace`
  - `GET /v1/sessions`
  - `GET /v1/sessions/{session_key}`
  - `GET /v1/sessions/{session_key}/history`
  - `GET /healthz`
  - `GET /readyz`
- Worker：
  - EventLoop
  - heartbeat worker
  - processing sweeper
  - delayed job worker
  - outbox retry worker
  - cron worker
- LLM：
  - 统一 `LLMProvider.Chat(ctx, ChatRequest)` / `StreamChat(ctx, ChatRequest, StreamSink)` 接口
  - 默认 `fake/default`
  - OpenAI-compatible provider 使用 `github.com/openai/openai-go/v3`
- `NO_REPLY`：`memory_flush | compaction | cron_fire`
- 文件系统仅保留：
  - `workspace/memory/**`
  - `workspace/runtime/native/**`
  - `workspace/runtime/app.db`

## Prompt / Skills / Memory

当前 runtime 会在每次 agent run 前构造一条 system message，并放在对话消息最前面。当前采用分层 prompt，固定 section 顺序如下：

- `Identity & Runtime Rules`
- `Tool Contract`
- `Memory Policy`
- `Workspace Instructions & Context`
- `Available Skills`
- `Heartbeat Policy`（仅 `cron_fire`）
- `Current Run Context`

### Workspace 提示文件

`init` 会在 workspace 根目录自动创建缺失的提示模板，但不会覆盖已有文件，也不会自动生成 `AGENTS.md`。

普通交互按顺序注入以下根文件：

- `SOUL.md`
- `IDENTITY.md`
- `USER.md`
- `AGENTS.md`
- `TOOLS.md`
- `BOOTSTRAP.md`（存在即注入）

`HEARTBEAT.md` 只会在 `payload.type=cron_fire` 时注入。

其中：

- `SOUL.md`：全局稳定人格与方法论
- `IDENTITY.md`：agent 身份设定
- `USER.md`：用户偏好、称呼、时区
- `AGENTS.md`：当前项目范围内的局部工作约定
- `TOOLS.md`：环境事实与工具可用性
- `BOOTSTRAP.md`：短期 onboarding 文件；完成初始化后应手动删除
- `HEARTBEAT.md`：后台巡检/整理 checklist，仅服务 `cron_fire`

### Skills

- skill 采用工作区内 Markdown 技能包：`workspace/skills/<name>/SKILL.md`
- prompt 中只注入紧凑的 skill 索引（`name / description / path`），不会注入 skill 正文
- 需要读取正文时，模型应使用 `context_get`

`context_get` 目前只允许读取 workspace 根目录固定上下文文件或 skill 正文：

- `SOUL.md`
- `IDENTITY.md`
- `USER.md`
- `AGENTS.md`
- `TOOLS.md`
- `BOOTSTRAP.md`
- `HEARTBEAT.md`
- `skills/<name>/SKILL.md`

### Memory

memory 采用双轴模型：

- `visibility`: `public | private`
- `kind`: `curated | daily`

canonical 路径如下：

- `memory/public/MEMORY.md`
- `memory/private/MEMORY.md`
- `memory/public/daily/YYYY-MM-DD.md`
- `memory/private/daily/YYYY-MM-DD.md`

兼容读取旧根路径 `MEMORY.md`，但所有新写入都走 canonical 路径。

默认策略：

- prompt 只注入 curated memory
- `public` curated 始终注入
- `private` curated 仅在 `dm` 会话中注入
- daily memory 默认不注入，优先通过 `memory_search` + `memory_get` recall

`memory_search` 现使用双轴过滤参数：

- `visibility=auto|public|private`
- `kind=any|curated|daily`

## 目录概览

- `cmd/simiclaw/internal/*`：命令入口
- `internal/bootstrap`：应用装配与生命周期
- `internal/channels`：CLI / Telegram 等接入适配
- `internal/gateway`：ingest 校验、限流与幂等边界
- `internal/httpapi`：HTTP 路由、handler、鉴权与分页
- `internal/memory`：工作区记忆读写
- `internal/outbound`：出站 sender
- `internal/provider`：LLM provider 抽象与实现
- `internal/runner`：执行编排
- `internal/runtime`：EventLoop、Supervisor、后台 workers
- `internal/session`：session key 归一化与计算
- `internal/store`：SQLite 启动、schema、读写与恢复
- `pkg/config`：配置模型
- `pkg/logging`：日志封装
- `pkg/model`：共享类型
- `pkg/tools`：未来 tools / skills 扩展边界

## 快速开始

### 1. 初始化 workspace

```bash
go run ./cmd/simiclaw init --workspace ./workspace
```

默认会自动 scaffold 缺失的 `SOUL.md`、`IDENTITY.md`、`USER.md`、`TOOLS.md`、`BOOTSTRAP.md`、`HEARTBEAT.md`，但不会覆盖已有文件，也不会自动创建 `AGENTS.md`。

若检测到旧文件式 runtime 痕迹，默认拒绝；只有显式传入 `--force-new-runtime` 才会清理 legacy 目录并创建新的 SQLite runtime。

### 2. 启动服务

若要接真实 OpenAI-compatible 模型，可先在仓库根目录放置 `.env`：

```bash
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.deepseek.com
LLM_MODEL=openai/deepseek-chat
```

兼容旧环境变量名 `LLM_API_KEY` / `LLM_BASE_URL`。若配置非法，`serve` 会直接启动失败，而不是静默退回 `fake/default`。

```bash
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080
```

### 3. 使用 chat CLI

```bash
go run ./cmd/simiclaw chat --base-url http://127.0.0.1:8080
```

`chat` 默认进入 Bubble Tea TUI：启动先选会话，可新建会话、回放历史、发送消息，并优先使用 `POST /v1/chat:stream` 流式展示回复；如果流中断或服务端不支持，会自动回退到 ingest + 轮询。

### 4. Inspect / Completion

```bash
go run ./cmd/simiclaw inspect health
go run ./cmd/simiclaw inspect sessions --limit 20
go run ./cmd/simiclaw inspect trace <run-id> --output json
go run ./cmd/simiclaw completion bash
```

### 5. 手动 ingest

```bash
NOW=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

curl -sS -X POST "http://127.0.0.1:8080/v1/events:ingest" \
  -H "Content-Type: application/json" \
  -d "{
    \"source\": \"cli\",
    \"conversation\": {
      \"conversation_id\": \"demo\",
      \"channel_type\": \"dm\",
      \"participant_id\": \"u1\"
    },
    \"idempotency_key\": \"cli:demo:1\",
    \"timestamp\": \"${NOW}\",
    \"payload\": {
      \"type\": \"message\",
      \"text\": \"hello\"
    }
  }"
```

## 测试与验收

```bash
make fmt
make vet
make lint
make test-unit
make test-unit-race-core
make test-integration
make test-e2e-smoke
make accept-v1-alpha
make accept-current
```

### `cron_fire` 行为

- 对外仍属于 `NO_REPLY`，最终 event 状态为 `suppressed`
- 内部会走 suppressed LLM + tool loop，而不是直接写 memory 后结束
- tool 权限显式限制为 `memory_search`、`memory_get`、`context_get`
- `cron_fire` 产生的入口消息、assistant 中间消息、tool 调用结果、最终 assistant 消息都会持久化为 hidden message
- 普通 UI 的默认历史查询不会显示这些消息，普通聊天恢复上下文时也不会回灌这些 `cron_fire` 历史

## 运行时约束

- `POST /v1/events:ingest` 必须显式提供 `idempotency_key`
- Gateway 事务提交前，事件不得入队
- 事件只有在领取事务成功后才能进入 `processing`
- 同一 event 任一时刻最多一个活跃 run
- 所有外部 I/O 都在写事务外执行
- 真实发送必须晚于 outbox 持久化提交
- FTS 只由 SQLite trigger 维护，应用层禁止双写
