# Data Model: Runtime Kernel Refactor

## Preserved Persisted Runtime Facts

这些对象的“存在意义”和持久化角色在本次重构中保持不变：

### Event Fact

- Source of truth: `events`
- Represents: 一次被 ingest 接收并进入 runtime 的事件
- Key identity: `event_id`
- Key invariants:
  - 只有 claim 成功后才能进入 `processing`
  - 事件执行最终由 finalize 写回终态
  - 与 `run_id`、`outbox_id`、`session_key` 形成主链路

### Run Fact

- Source of truth: `runs`
- Represents: 某个 event 的一次执行尝试
- Key identity: `run_id`
- Key invariants:
  - claim 事务内创建 `started`
  - finalize 事务内进入 `completed` 或 `failed`
  - 承载 provider/model/tokens/latency/diagnostics

### Message Fact

- Source of truth: `messages`
- Represents: user / assistant / tool 历史消息
- Key invariants:
  - 在 finalize 事务中统一插入
  - FTS 由 SQLite triggers 维护

### Outbox Fact

- Source of truth: `outbox`
- Represents: 持久化后的发送意图
- Key invariants:
  - 真实发送必须发生在 outbox intent 提交之后
  - 发送失败通过 retry / dead 状态推进

### Scheduled Job Fact

- Source of truth: `scheduled_jobs`
- Represents: cron / delayed / retry 的后台任务意图
- Key invariants:
  - 调度意图必须持久化
  - 任务处理与 event 处理共享 runtime 约束

### Session Projection

- Source of truth: `sessions`（派生缓存）
- Represents: 会话摘要、最近活跃、token 统计
- Key invariants:
  - 不是事实源
  - 由 finalize / projection 逻辑更新

## New Internal Runtime Kernel Entities

这些是本次重构要显式建模的内部对象，用来替代当前散落在中心文件里的隐式职责。

### Runtime Kernel

- Represents: 运行时内核 owner
- Responsibilities:
  - 接收 runnable event 或 background work
  - 协调 claim、execute、finalize、delivery intent
  - 管理 worker owner、生命周期、stop path 和 lane hooks

### Execution Request

- Represents: 一次准备执行的 runtime work item
- Fields:
  - work kind（event, outbox send, scheduled job, recovery）
  - identity（event_id / outbox_id / job_id）
  - lane key
  - metadata needed for routing to the correct handler

### Claim Context

- Represents: claim 成功后，执行层获得的最小上下文
- Fields:
  - claimed event or claimed job metadata
  - run id
  - run mode
  - session key / session id
  - source and channel metadata

### Execution Result

- Represents: 执行层的纯结果对象
- Fields:
  - output messages
  - assistant reply
  - trace / diagnostics
  - suppress output flag
  - delivery intent request
  - failure info

### Finalize Command

- Represents: 提交给事实适配器的一次 finalize 请求
- Purpose:
  - 统一表达 run、event、messages、outbox、session projection 的最终提交
  - 保持对 store 的 consumer-owned contract，而不是泄露 store 内部类型

### Delivery Envelope

- Represents: 提交后的发送任务
- Fields:
  - outbox identity
  - channel
  - target
  - body
  - retry policy / attempts snapshot

### Worker Role

- Represents: 一个具名后台职责，例如 heartbeat、processing recovery、delivery、cron scheduler
- Responsibilities:
  - own its poll interval
  - own its heartbeat name
  - own its stop path
  - expose retry / lease semantics

### Lane Key

- Represents: 并发车道标识
- Purpose:
  - 表达 session serialization 或 named queue 行为
  - 使 future concurrency 建立在显式 ownership 上

## Relationships

```text
Event Fact -> Claim Context -> Execution Result -> Finalize Command
Finalize Command -> Run Fact
Finalize Command -> Message Fact
Finalize Command -> Event Fact (terminal state)
Finalize Command -> Outbox Fact (optional)
Finalize Command -> Session Projection

Scheduled Job Fact -> Execution Request -> Claim Context
Outbox Fact -> Delivery Envelope -> Delivery Worker
Execution Request -> Lane Key -> Worker Role / Kernel scheduler
```

## Modeling Rules

- 持久化事实与内核运行对象必须分离建模。
- `store` 只消费 finalize/claim/delivery 等边界 DTO，不向上层泄露行级结构。
- HTTP 和 channel 边界不得直接拼装持久化事实；它们只生产 normalized ingress 或消费 delivery envelopes。
- 并发策略依赖 lane key 和 owner，而不是把 session order 藏在随机 goroutine 行为里。
