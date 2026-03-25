# Data Model: 按四块统一骨架重建 SimiClaw

## Persisted Runtime Facts That Stay Fixed

这些事实对象的持久化语义在本次重构中不改变：

### Event Fact

- Source of truth: `events`
- Represents: 一次被系统接受并进入 runtime 的 durable command 事件
- Key identity: `event_id`
- Key invariants:
  - 只有 claim 成功后才能进入 `processing`
  - 终态由 finalize 事务统一写回

### Run Fact

- Source of truth: `runs`
- Represents: 某个 event 的一次执行尝试
- Key identity: `run_id`
- Key invariants:
  - claim 事务内创建 `started`
  - finalize 事务内进入 `completed` 或 `failed`

### Message Fact

- Source of truth: `messages`
- Represents: user / assistant / tool 历史消息
- Key invariants:
  - 由 finalize 事务统一落库
  - FTS 仍只由 SQLite trigger 维护

### Outbox Fact

- Source of truth: `outbox`
- Represents: durable delivery intent
- Key invariants:
  - 真实发送只能发生在 outbox 提交之后
  - retry / dead-letter 语义继续停留在 delivery 边界

### Scheduled Job Fact

- Source of truth: `scheduled_jobs`
- Represents: cron / delayed / retry 等主动型任务意图
- Key invariants:
  - 调度意图必须持久化
  - 任务执行仍受 Runtime 两阶段语义约束

### Session Projection

- Source of truth: `sessions`（derived cache）
- Represents: 会话摘要和派生读模型
- Key invariants:
  - 不是事实源
  - 仅作为查询与体验优化的投影

## New Architectural Entities

### Surface Command Request

- Represents: Surface adapter 归一化后的 command 输入
- Typical fields:
  - auth / caller context
  - normalized payload
  - session hints
  - idempotency key
  - response mode hint
- Rules:
  - 只携带 transport-normalized 输入
  - 不携带 store row 或持久化控制细节

### Query Request

- Represents: Surface 发起的只读查询请求
- Typical fields:
  - query type
  - filter / pagination
  - caller scope
- Rules:
  - 不推进 durable state
  - 只读取 projections 或允许暴露的 facts 视图

### Observe Subscription

- Represents: Surface 建立的 runtime observe 订阅
- Typical fields:
  - stream target
  - replay anchor
  - session / event scope
- Rules:
  - transport 只消费 runtime events
  - Surface 不拥有 observe event hub 本体

### Runtime Command Envelope

- Represents: 进入 Runtime 的 durable command 表达
- Typical fields:
  - event identity
  - run mode
  - lane/session ownership key
  - routing metadata
- Rules:
  - 只能由批准的 command ingress 生成
  - 是 Runtime 的执行起点，不是 Surface 的 transport 对象

### Claim Context

- Represents: claim 成功后，Runtime 执行链路持有的最小上下文
- Typical fields:
  - `event_id`
  - `run_id`
  - session metadata
  - payload type
  - execution mode

### Context Bundle

- Represents: Runtime 执行所需的只读上下文集合
- Sources:
  - prompt assets
  - memory
  - workspace context
  - projections/query summaries
- Rules:
  - 不是 runtime facts 的替代来源
  - 不直接拥有状态推进能力

### Capability Invocation

- Represents: Runtime 对 tool / provider / skill / MCP / router 的一次调用
- Typical fields:
  - capability kind
  - timeout / retry policy
  - request payload summary
  - response / error summary
- Rules:
  - 失败语义在 Runtime 与 Capability 边界显式表达
  - Capability 不直接写 durable state

### Execution Result

- Represents: 事务外执行得到的纯结果
- Typical fields:
  - assistant output
  - tool results
  - trace / diagnostics
  - delivery intents
  - failure summary

### Host Control Action

- Represents: 进程内 supervision / lifecycle 控制动作
- Examples:
  - start worker
  - stop runtime host
  - readiness probe aggregation
- Rules:
  - 只允许 process-local control 直接走 host control
  - 改 durable state 的 admin 动作必须回到 durable command path

## Relationships

```text
Surface Command Request -> Runtime Command Envelope -> Claim Context -> Context Bundle + Capability Invocation -> Execution Result -> Finalize Transaction

Finalize Transaction -> Event Fact
Finalize Transaction -> Run Fact
Finalize Transaction -> Message Fact
Finalize Transaction -> Outbox Fact
Finalize Transaction -> Session Projection

Query Request -> Projection / Read Model
Observe Subscription -> Runtime Events / Replay
Host Control Action -> Runtime Host / Workers
```

## Modeling Rules

- 持久化 facts、只读 projections、上下文 assets、capability calls 必须分开建模。
- Runtime 通过 consumer-owned interface 读取 facts/projections/context，不能重新长出 god repository。
- Surface 请求对象只描述 transport-normalized 输入，不能夹带 state transition 细节。
- Capability 调用只返回结果或错误，不推进 durable state。
