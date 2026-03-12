# Core Beliefs

## Why This Exists

SimiClaw 处在 `V1` 阶段，系统仍在快速收敛，但几条核心设计取舍已经相当稳定。把这些原则单独写出来，可以减少在 README、评审和新功能设计里反复解释同一组理由。

## Beliefs

1. SQLite-first 比文件散落式 runtime 更重要。
   运行态事实统一收敛到 `workspace/runtime/app.db`，这让幂等、查询、恢复、后台 worker 和 trace 都有同一个事实源。
2. 接收写入和执行写入必须分离。
   ingest 的职责是验证、持久化和排队；真正的 LLM / tool 执行发生在 EventLoop claim 成功之后，避免把长时执行塞进写事务。
3. 边界要靠测试，而不只靠口头约定。
   `tests/architecture/boundaries_test.go` 直接限制不允许的 import 和类型泄漏，让“窄接口、分层依赖”成为可执行规则。
4. Prompt 是分层上下文，不是随意拼接文本。
   系统固定 prompt、curated memory、workspace 文件、skills 索引和当前 run context 各自承担不同责任，顺序稳定，便于调试和复现。
5. Workspace 文件操作必须是显式能力，不是任意文件系统访问。
   `workspace_patch` / `workspace_delete` 与 `workspacefile` 共同限制路径、文本类型和私有内存访问，避免运行时越界写文件。

## Implications

- 不应重新引入 `sessions.json`、`outbound_spool` 这类文件式 runtime 事实源。
- 新功能如果需要写路径，优先接入 ingest / event loop，而不是旁路写库。
- 新子系统若要跨层传递数据，应先判断它是 `pkg/api` 对外契约，还是某个 `internal/<subsystem>/model` 的局部 DTO。
- 修改 prompt 或 workspace 行为时，应同时更新相应设计文档，避免事实重新散落回 README。

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 运行链路: [`runtime-flow.md`](runtime-flow.md)
- 模块边界: [`module-boundaries.md`](module-boundaries.md)
- Prompt / Workspace: [`prompt-and-workspace-context.md`](prompt-and-workspace-context.md)
