# Quickstart: 验证日志升级

以下步骤用于在实现完成后快速验证“日志格式变为可读文本，且主链路日志点已补齐”。

## 1. 启动服务

```bash
go run ./cmd/simiclaw serve --workspace ./workspace
```

预期：

- 启动日志为单行可读文本，而不是 JSON-like 字段块。
- 能看到 `serve` 启动相关日志，但不会出现高频空转刷屏。

## 2. 发送一次最小 ingest 请求

```bash
TIMESTAMP="$(date -u +"%Y-%m-%dT%H:%M:%SZ")"

curl -sS http://127.0.0.1:8080/v1/events:ingest \
  -H 'Content-Type: application/json' \
  -d "{
    \"source\": \"manual\",
    \"conversation\": {
      \"conversation_id\": \"demo-conv\",
      \"channel_type\": \"cli\"
    },
    \"idempotency_key\": \"demo-log-upgrade-1\",
    \"timestamp\": \"${TIMESTAMP}\",
    \"payload\": {
      \"type\": \"message\",
      \"text\": \"hello logging\"
    }
  }"
```

如果服务端配置了 `api_key`，需要额外携带 `-H 'Authorization: Bearer <api_key>'`。

预期日志至少覆盖：

- ingest accepted / duplicate / rejected 之一
- enqueue outcome（包括 accepted but not enqueued）
- runtime claim/start
- finalize completed 或 failed
- outbound sent / retry scheduled / dead-lettered 结果

## 3. 人工检查字段与级别

重点检查：

- 同一条链路是否能通过 `event_id`、`run_id`、`session_key` 串起来。
- `duplicate`、`rate limited`、`invalid argument` 等拒绝类结果没有被一律打成 `ERROR`。
- `info` 级别没有出现 keepalive、空轮询、逐 chunk 明细。

## 4. 人工检查脱敏

重点检查：

- 日志中不应出现 API key、Bearer token、Telegram token。
- 日志中不应直接打印完整 prompt、private memory 内容或大型 tool payload 原文。
- tool / provider 相关日志应显示摘要信息而不是原始大对象。

## 5. 回归验证命令

```bash
make fmt
go test ./pkg/logging/...
go test ./tests/architecture/... -v
make test-unit
make accept-current
```

## 6. 最近一次验证记录

- `2026-03-20`: `make fmt` 通过
- `2026-03-20`: `go test ./pkg/logging/...` 通过
- `2026-03-20`: `go test ./tests/architecture/... -v` 通过
- `2026-03-20`: `make test-unit` 通过
- `2026-03-20`: `make accept-current` 通过
