# Research: Runtime Kernel Refactor

## Decision 1: 保留事实模型与运行不变量，不做存储重平台

- **Decision**: 保留现有 SQLite-first 事实模型与 `claim -> execute -> finalize` 运行不变量，不以 PostgreSQL、消息队列或多进程调度替换当前事实源。
- **Why**: 当前项目最有价值的是已经收敛出来的幂等、恢复、持久化和 durable delivery 语义。全量重平台会重新引入大量不确定性。
- **Alternatives considered**:
  - 全量重写后端并重新设计存储与执行模型：被拒绝，因为会同时丢掉现有运行语义资产和验证基础。
  - 保持现状仅继续堆功能：被拒绝，因为当前结构已经不适合承载后续 s01-s10 的持续演进。

## Decision 2: 采用“kernel-first, boundaries-later” 的迁移顺序

- **Decision**: 第一阶段先重建 runtime kernel 和扩展接口，再重构 gateway/routing，之后再重挂 HTTP/channels/delivery，最后引入 concurrency lanes。
- **Why**: 现在真正缺的是稳定内核，而不是少几个外部边界实现。先重构外部边界会把旧结构的不清晰继续扩散。
- **Alternatives considered**:
  - 先保 HTTP/CLI/Telegram 兼容再重构内核：被拒绝，因为会迫使新内核受旧接口形状牵制。
  - 先做 concurrency：被拒绝，因为在没有清晰 owner 和契约之前并发设计只会放大混乱。

## Decision 3: 保留顶层子系统名称，优先重组包内形状

- **Decision**: 保留 `internal/runtime`、`internal/gateway`、`internal/channels`、`internal/outbound`、`internal/store` 等顶层子系统名称，先重组其内部结构和 contracts，再视需要引入子包。
- **Why**: 顶层子系统和架构测试已经形成稳定认知，直接重命名所有核心包会增加迁移噪声。
- **Alternatives considered**:
  - 新建完全不同的顶层包并整体替换：被拒绝，因为会同时放大架构测试、文档和所有导入点的迁移成本。

## Decision 4: 把 HTTP 与 channels 视为 replaceable boundaries，而非第一阶段兼容边界

- **Decision**: HTTP、CLI、Telegram 等外部边界当前不是第一阶段必须保形的边界；它们应以新 contracts 为准重新挂接。
- **Why**: 用户已明确这轮重构不以保当前接口形状为优先，而以可扩展内核为优先。
- **Alternatives considered**:
  - 把现有所有外部边界当不可动兼容边界：被拒绝，因为会显著限制新 contracts 的设计空间。

## Decision 5: 并发能力以后续 lane-ready 设计落地，而不是和 kernel slice 绑定交付

- **Decision**: 在 kernel slice 中仅引入 future-facing lane hooks 和 ownership model，不在第一阶段交付完整 session lane serialization。
- **Why**: 当前最急需的是明确 owner、worker lifecycle、delivery intent 和 execution contracts；完整并发策略属于下一层复杂度。
- **Alternatives considered**:
  - 第一阶段直接上 session lanes：被拒绝，因为会把结构重构和并发正确性问题耦合到一起。

## Resulting Architectural Direction

- `store` 继续是 SQLite 事实适配器，不再被视为全系统的隐式中心。
- `runtime` 成为显式 kernel owner，负责执行生命周期、worker coordination 和 durable intents。
- `gateway` 承接路由与会话绑定规则，`http` 与 `channels` 在各自边界完成输入 normalize，`outbound` 承接 delivery policies。
- 所有未来能力都以扩展点接入，而不是在中心流程里继续增加分支。
