# Research: 按四块统一骨架重建 SimiClaw

## Decision 1: 四块是 owner map，不是强制的新顶层目录重命名工程

- **Decision**: 采用 `Surface`、`Runtime`、`Context/State`、`Capability Plane` 作为架构 owner map，但优先保留当前已经有稳定认知的顶层包名，只重写职责边界、依赖方向和中心对象形状。
- **Rationale**: 本次真正要解决的是 owner 混乱和控制流耦合，而不是路径名字本身。若先做大规模目录迁移，会放大噪声并掩盖行为风险。
- **Alternatives considered**:
  - 新增四个全新顶层目录并整体搬家：被拒绝，因为目录重命名本身不等于职责分离，且会显著放大迁移面。

## Decision 2: `command/query/observe` 作为显式 seam，而不是第五个顶层平面

- **Decision**: 让 `command/query/observe` 成为 Surface 调用的显式用例边界，但不再引入单独的第五个 owner plane。
- **Rationale**: 用户 spec 已经明确四块才是顶层 owner。若再升出第五层，会把“边界 seam”误做成新的所有权中心。
- **Alternatives considered**:
  - 新建统一 `internal/app` 或 `internal/usecase` 作为第五平面：被拒绝，因为会把很多原本应该属于 Runtime 或 Query 的逻辑再次集中到新中枢。

## Decision 3: 默认冻结现有外部可见契约

- **Decision**: 本次架构重构默认冻结现有 HTTP、SSE、CLI 与 Web 可观察契约；任何 wire contract 变化都需要单独 spec。
- **Rationale**: 仓库的 `pkg/api` 已经被定义为稳定外部契约，且当前 spec 的目标是内部骨架重建，不是产品行为翻新。
- **Alternatives considered**:
  - 借重构顺手调整 `chat:stream` 或其他外部契约：被拒绝，因为这会让验证边界和回滚边界失焦。

## Decision 4: `web/` 是 Surface 契约消费者，不是后端 adapter owner

- **Decision**: `web/` 在本次计划中只作为消费 HTTP/stream/query contract 的客户端校验面，而不作为后端 Surface adapter owner 参与 Runtime 边界设计。
- **Rationale**: `web/` 不负责 auth、request normalization、durable command ingest 或 runtime observe publication；它消费这些能力，而不是拥有这些能力。
- **Alternatives considered**:
  - 把 `web/` 与 `internal/http/`、`cmd/simiclaw/`、`internal/channels/` 并列视为同类后端 adapter owner：被拒绝，因为会模糊 client 与 server owner 的边界。

## Decision 5: 迁移切片按“先 guardrail，后实现；先 Runtime/Capability，后 Surface”推进

- **Decision**: 实施顺序固定为 guardrails/contract freeze -> Runtime/Capability core decomposition -> Context/State separation -> Surface adapter convergence -> bootstrap/docs cleanup。
- **Rationale**: 不先钉住 guardrails，就很容易把新设计再次长成中心对象；不先拆 Runtime/Capability，Surface 再怎么改也只是外壳换皮。
- **Alternatives considered**:
  - 先改 Surface adapter，再倒逼内部重组：被拒绝，因为会让旧中心对象继续主导设计。
  - 一次性全仓替换：被拒绝，因为不符合宪章要求的可回滚、可分阶段迁移。

## Decision 6: 每个 slice 都必须同时完成“新 owner 接管 + 旧路径删除”

- **Decision**: 任何被接受的迁移切片都必须在同一变更里完成 owner 接管和旧路径清理，不保留长期并存的双路径。
- **Rationale**: 用户与仓库规则都明确不接受兼容层、双写或过渡性桥接逻辑。对架构重构而言，最危险的状态就是“新旧两套都在但谁都不完整”。
- **Alternatives considered**:
  - 长期保留新旧 API 或新旧 owner 并行：被拒绝，因为会迅速退化为不再可解释的混合架构。

## Decision 7: 当前 `chat:stream` 组合行为按 4 个 owner 拆解

- **Decision**: 对 `POST /v1/chat:stream` 这类组合入口，command ingest 归 command boundary，runtime event publication/replay 归 Runtime，SSE frame 组装归 Surface stream adapter，CLI/Web 的 retry 或 polling fallback 归各自 client consumer。
- **Rationale**: 当前最危险的不是“有一个组合入口”，而是它在同一模块里同时拥有 transport、执行、observe 和 fallback 语义。把组合入口保留在表层是可以的，但 owner 必须拆开。
- **Alternatives considered**:
  - 让后端 stream handler 同时继续承担 query fallback：被拒绝，因为这会把混合中心对象换个位置继续保留。
  - 新建一个统一的超大 use-case 包收口所有流式行为：被拒绝，因为容易演化成新的 god layer。
