# SimiClaw (v0.4)

一个简化版、单机单进程的 OpenClaw 风格 Agent Runtime，使用 Go 实现，当前阶段为 `M4`。

- 当前阶段：`M4`（见 `VERSION_STAGE`）
- 架构核心：`Gateway + EventLoop + StoreLoop`
- 持久化核心：`runtime/sessions.json + runtime/sessions/<session_id>.jsonl + runtime/runs + runtime/events + runtime/idempotency`

## 当前能力（M4）

- HTTP 入站：`POST /v1/events:ingest`
- 事件查询：
  - `GET /v1/events/{event_id}`
  - `GET /v1/events`
  - `GET /v1/events:lookup`
- 运行查询：
  - `GET /v1/runs`
  - `GET /v1/runs/{run_id}`
  - `GET /v1/runs/{run_id}/trace?view=&redact=`
- 会话查询：
  - `GET /v1/sessions`
  - `GET /v1/sessions/{session_key}`
- 审批接口：
  - `POST /v1/approvals`
  - `POST /v1/approvals/{approval_id}:approve`
  - `POST /v1/approvals/{approval_id}:reject`
  - `GET /v1/approvals/{approval_id}`
  - `GET /v1/approvals`
- 健康检查：`GET /healthz`、`GET /readyz`
- 入站幂等（`idempotency_key`）
- 会话路由（`session_key`）与 `active_session_id` 解析
- 单写者提交顺序保证：
  1. 追加 `sessions/<session_id>.jsonl`
  2. 写 `runs/<run_id>.json`
  3. 更新 `sessions.json`
- commit 成功后再出站发送
- `NO_REPLY` 事件（如 `memory_flush`）抑制用户可见出站
- 内置记忆工具：
  - `memory_search`（scope/top_k 检索）
  - `memory_get`（路径白名单 + 行范围 + 字符上限）
- 记忆写入动作：
  - daily：`workspace/memory/YYYY-MM-DD.md`
  - curated：`workspace/MEMORY.md`
- compaction 内务回合：
  - `payload.type=compaction|memory_flush|cron_fire` 自动 NO_REPLY
  - run trace 写入 `context_manifest/rag_hits/tool_executions`
- 审批与演化：
  - 高风险 `Patch` 动作自动进入 `pending approval`
  - approve/reject 回流 `approval_granted/approval_rejected`
  - Patch 支持 `expected_base_hash` 与 `patch_idempotency_key`
  - Patch 失败自动回滚，目标文件保持稳定版本
  - evolution 日报输出到 `workspace/evolution/YYYY-MM-DD.md`
- 崩溃恢复：
  - `sessions.json` 不可读/缺失时可由 `runtime/sessions/*.jsonl` 重建索引
  - 尾部半截 batch（缺 commit marker）会在启动时丢弃未提交尾部

更多细节见 [doc/m4-capabilities.md](./doc/m4-capabilities.md)。

## 快速开始

### 1. 环境要求

- Go `1.25+`（见 `go.mod`）

### 2. 初始化 workspace

```bash
go run ./cmd/simiclaw init --workspace ./workspace
```

会生成（若不存在）：

- `workspace/runtime/sessions.json`
- `workspace/runtime/sessions/`
- `workspace/runtime/runs/`
- `workspace/runtime/events/events.json`
- `workspace/runtime/idempotency/*.jsonl`
- `workspace/runtime/cron/jobs.json`

### 3. 启动服务

```bash
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080
```

也可使用同义命令：

```bash
go run ./cmd/simiclaw gateway --workspace ./workspace --listen :8080
```

### 4. 使用 chat CLI（推荐）

在另一个终端运行：

```bash
go run ./cmd/simiclaw chat
```

可选参数（精简）：

- `--base-url`：网关地址，默认 `http://127.0.0.1:8080`
- `--conversation`：会话 ID，默认 `cli_default`
- `--api-key`：当服务端配置了 `api_key` 时传入

输入 `/quit` 或 `/exit` 退出。

### 5. 手动调用 ingest（可选）

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

### 6. 查询事件状态

将上一步响应里的 `event_id` 带入：

```bash
curl -sS "http://127.0.0.1:8080/v1/events/<event_id>"
```

常见结果：

- `status=committed` + `delivery_status=sent`：主链路成功
- `status=failed`：处理失败，可查看 `error`

## 幂等与重试说明

- 同一个 `idempotency_key` 重复请求：
  - payload 一致：返回 duplicate（不会重复执行）
  - payload 不一致：返回 `409 CONFLICT`
- 当请求在“入队前”失败时，服务端会回滚该次幂等登记，可以使用同 key 重试

## 测试与验收

```bash
make fmt                 # 格式化 Go 代码
make vet                 # 运行 go vet 进行基础静态语法检查
make lint                # 运行 linter (如 golangci-lint) 进行代码规范检查
make test-unit           # 运行所有基础单元测试
make test-unit-race-core # 运行带数据竞争检测 (-race) 的核心逻辑单元测试
make test-integration    # 运行集成测试，验证各模块（如入站到存储）的交互
make test-e2e-smoke      # 运行端到端 (E2E) 冒烟测试，验证核心链路可用性
make accept-current      # 自动执行当前开发阶段的完整自动化验收流程
```

`accept-current` 会根据 `VERSION_STAGE` 自动选择当前阶段验收入口。

## 项目文档

- [doc/requirements.md](./doc/requirements.md)
- [doc/architecture.md](./doc/architecture.md)
- [doc/interfaces.md](./doc/interfaces.md)
- [doc/roadmap.md](./doc/roadmap.md)
- [doc/cicd-testing.md](./doc/cicd-testing.md)
- [doc/m1-capabilities.md](./doc/m1-capabilities.md)
- [doc/m2-capabilities.md](./doc/m2-capabilities.md)
- [doc/m3-capabilities.md](./doc/m3-capabilities.md)
- [doc/m4-capabilities.md](./doc/m4-capabilities.md)
