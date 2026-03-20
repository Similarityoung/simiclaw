# Configuration Reference

## Summary

本文汇总服务端配置、已实现的环境变量覆盖、CLI 运行参数和 HTTP 鉴权行为。所有字段都以代码为准，尤其是 `internal/config/config.go`、`cmd/simiclaw/internal/gateway/command.go` 和 `cmd/simiclaw/internal/common/cli.go`。

## 服务端加载顺序

`serve` 命令的配置路径如下：

1. 尝试加载仓库根目录 `.env`，只在当前环境变量缺失时补值
2. 读取 `--config` 指向的 JSON 配置文件；未提供则从 `config.Default()` 起步
3. 应用服务端环境变量覆盖
4. 再用 `--workspace` 与 `--listen` 覆盖最终值

## 主要配置字段

| 字段 | 作用 | 默认值 / 约束 |
| --- | --- | --- |
| `workspace` | 工作区根目录 | `"."` |
| `listen_addr` | HTTP 监听地址 | `:8080` |
| `log_level` | 日志级别 | `info`，仅允许 `debug\|info\|warn\|error` |
| `api_key` | HTTP Bearer 鉴权口令 | 默认空；非空时保护所有 `/v1/**` 路由 |
| `tenant_id` | session key 与限流维度 | `local` |
| `event_queue_capacity` | EventLoop 队列长度 | `1024` |
| `ingest_enqueue_timeout` | ingest 入队超时 | `200ms` |
| `rate_limit_tenant_rps` / `burst` | tenant 级限流 | `30 / 60` |
| `rate_limit_session_rps` / `burst` | session 级限流 | `5 / 10` |
| `max_tool_rounds` | 最大工具回合数 | `4` |
| `db_busy_timeout` | SQLite busy timeout | `5s`，且必须 `>= 1s` |
| `llm.default_model` | 默认 provider/model | 默认 `fake/default`，必须是 `provider/model` 形式 |
| `llm.providers.*` | provider 配置 | 支持 `fake` 与 `openai_compatible` |
| `web_search` | 内置 `web_search` 超时与结果数 | 默认 `10s` / `5`，结果数会被 clamp 到 `1..8` |
| `channels.telegram` | Telegram long polling 配置 | 启用时必须提供 token |
| `cron_jobs[]` | 后台定时 fire 配置 | `name`、`interval`、`conversation_id`、`channel_type`、`payload_type` 必填 |

## 日志行为

- 运行日志默认输出到 stdout/stderr，底层采用 zap console / development 风格的单行文本格式。
- 每条日志保留时间、级别、caller 和 `[module] message` 前缀；附加上下文字段显示在行尾结构化对象中，而不是仓库自定义的 `key=value` 文本字段区。
- 当 stdout 或 stderr 是交互式终端时，级别会带 development 风格颜色；重定向到文件、pipe 或测试捕获时默认不输出 ANSI 颜色。
- JetBrains/GoLand 的 `#gosetup` 临时运行二进制会被视为支持 ANSI 颜色，因此从 IDE 的 Run Console 直接启动服务时也会显示彩色级别。
- 可通过 `SIMICLAW_FORCE_COLOR=1` 强制开启颜色，通过 `SIMICLAW_NO_COLOR=1` 显式关闭颜色。
- `log_level=info` 主要用于启动、ingest、runtime、finalize、outbound 等里程碑日志；`debug` 才允许暴露更高频的内部调度细节。
- HTTP 鉴权失败、参数校验失败、rate limit、duplicate、queue 未入队等拒绝或降级路径会按语义落到 `warn` 或 `info`，不会统一抬升成 `error`。
- 日志不会直接打印 API key、Bearer token、Telegram token；大型或复杂字段会被转成可读摘要或转义后的单值。
- `runner` / `runner.tool` / `runtime.worker` 等 owner 会补充 `provider`、`model`、`tool_name`、`worker` 等上下文；tool 参数与结果默认只输出摘要字段，不直接打印原始大对象。

## 服务端环境变量

| 环境变量 | 作用 |
| --- | --- |
| `SIMICLAW_LLM_DEFAULT_MODEL` | 覆盖 `llm.default_model` |
| `OPENAI_API_KEY` / `LLM_API_KEY` | 写入 `llm.providers.openai.api_key` |
| `OPENAI_BASE_URL` / `LLM_BASE_URL` | 写入 `llm.providers.openai.base_url` |
| `LLM_MODEL` | 覆盖默认模型 |
| `TELEGRAM_ENABLED` | 启用 Telegram channel |
| `TELEGRAM_TOKEN` | Telegram bot token |
| `TELEGRAM_ALLOWED_USER_IDS` | Telegram 允许用户 ID 列表，逗号分隔 |
| `TELEGRAM_LONG_POLL_TIMEOUT` | Telegram long polling 超时 |
| `WEB_SEARCH_TIMEOUT` | 内置 `web_search` 超时 |
| `WEB_SEARCH_MAX_RESULTS` | 内置 `web_search` 最大结果数 |

注：从当前代码看，服务端 `api_key` 只暴露了 JSON 字段，没有独立的服务端环境变量覆盖。

## CLI 运行参数与环境变量

`chat` 与 `inspect` 共用一套 runtime 选项解析：

| 旗标 / 环境变量 | 作用 | 默认值 |
| --- | --- | --- |
| `--base-url` / `SIMICLAW_BASE_URL` | 服务端基地址 | `http://127.0.0.1:8080` |
| `--api-key` / `SIMICLAW_API_KEY` | 客户端 Bearer token | 空 |
| `--timeout` / `SIMICLAW_TIMEOUT` | 请求超时 | `10s` |
| `--output` / `SIMICLAW_OUTPUT` | 输出格式 `table\|json` | 终端默认 `table`，非终端默认 `json` |
| `--no-color` / `SIMICLAW_NO_COLOR` / `NO_COLOR` | 关闭颜色输出 | `false` |
| `--verbose` / `SIMICLAW_VERBOSE` | 打开详细模式 | `false` |

## 鉴权行为

- `GET /healthz` 和 `GET /readyz` 始终可匿名访问。
- 当 `cfg.APIKey` 为空时，所有 `/v1/**` 路由都不做鉴权。
- 当 `cfg.APIKey` 非空时，客户端必须携带 `Authorization: Bearer <api_key>`。

## Verification

- 配置模型与默认值: `internal/config/config.go`
- `serve` 加载顺序: `cmd/simiclaw/internal/gateway/command.go`
- HTTP 鉴权: `internal/http/middleware/api_key.go`
- CLI 运行参数: `cmd/simiclaw/internal/common/cli.go`

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 测试矩阵: [`testing.md`](testing.md)
- 工作区布局: [`workspace-layout.md`](workspace-layout.md)
