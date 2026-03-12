# SimiClaw

单机单进程 Go agent runtime，当前阶段为 `V1`。系统以 SQLite 为唯一事实源，默认数据库位于 `workspace/runtime/app.db`；`sessions` 只是派生缓存，不是事实表。

## 先看哪里

- 系统总览: [`ARCHITECTURE.md`](ARCHITECTURE.md)
- 文档首页: [`docs/index.md`](docs/index.md)
- Agent 导航图: [`AGENTS.md`](AGENTS.md)

## 快速开始

1. 初始化工作区

```bash
go run ./cmd/simiclaw init --workspace ./workspace
```

2. 可选：在仓库根目录放置 `.env`

```bash
OPENAI_API_KEY=your-api-key
OPENAI_BASE_URL=https://api.deepseek.com
LLM_MODEL=openai/deepseek-chat
```

兼容旧别名 `LLM_API_KEY` / `LLM_BASE_URL`。

3. 启动服务

```bash
go run ./cmd/simiclaw serve --workspace ./workspace --listen :8080
```

4. 使用交互式 CLI

```bash
go run ./cmd/simiclaw chat --base-url http://127.0.0.1:8080
```

5. 读取健康与运行信息

```bash
go run ./cmd/simiclaw inspect health
go run ./cmd/simiclaw inspect sessions
go run ./cmd/simiclaw inspect runs
```

6. 启动前端

```bash
cd web
npm install
npm run dev
```

## 当前形态

- CLI 子命令: `init`, `serve` (`gateway` 别名), `chat`, `inspect`, `version`, `completion`
- HTTP 入口: `POST /v1/chat:stream`, `POST /v1/events:ingest`，以及 events / runs / sessions 查询接口
- 执行核心: `gateway -> ingest -> event loop -> runner -> store/outbox/query`
- LLM: 统一 provider 工厂，默认 `fake/default`，支持 OpenAI-compatible 配置
- 工作区上下文: scaffold 模板、curated memory、skills 索引和可控的 workspace 文件写接口

## 仓库地图

- `cmd/simiclaw/`: Cobra CLI 入口
- `internal/`: runtime、存储、prompt、tools、HTTP、channels 等核心实现
- `pkg/`: 对外稳定模型与日志封装
- `web/`: React + Vite 前端
- `tests/`: architecture / integration / e2e
- `docs/`: 架构、设计索引、参考资料、执行计划、质量评分

## 开发命令

```bash
make fmt
make lint
make test-unit
make accept-current
```

更完整的测试矩阵见 [`docs/references/testing.md`](docs/references/testing.md)，配置和环境变量见 [`docs/references/configuration.md`](docs/references/configuration.md)。
