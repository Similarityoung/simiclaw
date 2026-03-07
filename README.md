# SimiClaw (v1.0 alpha)

单机单进程 Go Agent Runtime，当前阶段为 `V1_ALPHA`。运行时采用 `SQLite-first` 架构，数据库固定为 `workspace/runtime/app.db`，`sessions` 仅作为派生缓存，SQLite 是唯一事实源。

## 当前能力

- CLI：`init | serve | chat | version`
- HTTP：
  - `POST /v1/events:ingest`
  - `GET /v1/events/{event_id}`
  - `GET /v1/events`
  - `GET /v1/events:lookup`
  - `GET /v1/runs`
  - `GET /v1/runs/{run_id}`
  - `GET /v1/runs/{run_id}/trace`
  - `GET /v1/sessions`
  - `GET /v1/sessions/{session_key}`
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
  - 统一 `LLMProvider.Chat(ctx, ChatRequest)` 接口
  - 默认 `fake/default`
  - OpenAI-compatible provider 使用 `github.com/openai/openai-go/v3`
- `NO_REPLY`：`memory_flush | compaction | cron_fire`
- 文件系统仅保留：
  - `workspace/memory/**`
  - `workspace/runtime/native/**`
  - `workspace/runtime/app.db`

## 快速开始

### 1. 初始化 workspace

```bash
go run ./cmd/simiclaw init --workspace ./workspace
```

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
go run ./cmd/simiclaw chat
```

### 4. 手动 ingest

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

## 运行时约束

- `POST /v1/events:ingest` 必须显式提供 `idempotency_key`
- Gateway 事务提交前，事件不得入队
- 事件只有在领取事务成功后才能进入 `processing`
- 同一 event 任一时刻最多一个活跃 run
- 所有外部 I/O 都在写事务外执行
- 真实发送必须晚于 outbox 持久化提交
- FTS 只由 SQLite trigger 维护，应用层禁止双写
