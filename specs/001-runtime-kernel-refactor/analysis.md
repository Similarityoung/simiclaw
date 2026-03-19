# Analysis: Runtime Kernel Refactor

## Purpose

本分析用于回答两个问题：

- 当前重构计划是否符合“先划边界、按职责分层、面向接口、目录即架构”的范式
- 在这个前提下，可以从 PicoClaw 的目录结构借鉴什么，又不能照搬什么

参考仓库：

- `sipeed/picoclaw` 仓库首页与目录树
- `docs/`、`cmd/`、`pkg/` 子目录结构

## What PicoClaw Gets Right

从目录层面看，PicoClaw 有几个很值得借鉴的优点：

### 1. 顶层结构一眼能读懂

它的顶层目录比较克制，`cmd`、`config`、`docs`、`pkg`、`web`、`workspace` 各自意图明确。读者不用先理解实现细节，就能知道：

- `cmd` 是程序入口
- `config` 是配置样例
- `docs` 是设计与迁移文档
- `web` 是前端
- `workspace` 是运行时工作区

这类“先看目录就知道系统大概长什么样”的能力，正是我们这次重构要恢复的。

### 2. 业务能力是一级目录，而不是埋在中心文件里

PicoClaw 的 `pkg/` 下面把很多能力直接拉成显式模块，例如：

- `agent`
- `channels`
- `gateway`
- `routing`
- `providers`
- `memory`
- `session`
- `tools`
- `heartbeat`
- `cron`
- `state`

这说明它在“能力 discoverability”上做得很好。新增或排查某类能力时，维护者可以先从目录找 owner，而不是先从一个巨大的 `service.go` 或 `runtime.go` 猜入口。

### 3. 文档也按主题分层

PicoClaw 的 `docs/` 下把设计、迁移、channels 等主题拆开了，而不是把所有说明塞进一份总文档。这和我们已经建立的 `AGENTS.md -> docs/ -> specs/` 结构是一致方向。

## What We Should Not Copy Directly

虽然 PicoClaw 的目录形状很清爽，但 SimiClaw 不能直接照搬，原因在于两者的核心约束不同。

### 1. 我们不是普通 agent app，而是 SQLite-first runtime

SimiClaw 的真正核心不是“功能清单”，而是运行不变量：

- SQLite 是唯一事实源
- 只有 claim 成功后才能进入 `processing`
- 执行必须是 `claim -> 事务外 execute -> finalize`
- 真实发送必须晚于 outbox intent 持久化

所以我们不能只追求“目录好看”，还必须让目录把这些事实边界表达出来。

### 2. 我们需要更强的 `internal` 边界

PicoClaw 的很多能力直接铺在 `pkg/` 下，这对发现性很好，但对我们来说过于宽松。SimiClaw 当前已经有明确约束：

- `internal/store` 不能向上泄露实现细节
- `runtime`、`gateway`、`channels`、`http`、`runner`、`query` 等层不能直接穿透到 `store`

因此我们更适合借鉴“按能力分包”的思想，而不是直接照搬“所有能力平铺到公共包”的形状。

### 3. 我们不能把运行语义藏进泛化命名

像 `bus`、`state`、`utils` 这样的名字，在参考项目里未必有问题，但对本仓库这轮重构不是好方向。我们更需要显式表达：

- 谁负责 ingest normalization
- 谁负责 claim/finalize
- 谁负责 durable delivery
- 谁负责 worker lifecycle
- 谁负责 lane ownership

否则只是把“代码写成一坨”换成“目录名看起来分散，但职责还是糊的”。

## Fit Against Our Refactor Principles

对照本次 speckit 计划，我的判断是：

### 已经符合的点

- 有明确 domain modeling：
  - `Event Fact`、`Run Fact`、`Outbox Fact`
  - `Runtime Kernel`、`Execution Request`、`Finalize Command`
- 有显式 contracts：
  - runtime execution
  - fact store boundary
  - payload handling
  - external ingress/egress
  - background workers
  - concurrency lanes
- 有分阶段迁移路线：
  - kernel
  - gateway/routing
  - http/channels/delivery
  - concurrency lanes

### 还需要补强的点

- “按能力分包”的表达还不够强，目前 plan 更像“技术切片”，还要进一步长成“能力 owner”
- 依赖方向虽然写了，但还可以更明确成：
  - HTTP 和 channel 边界只做协议转换与统一 ingress/egress
  - kernel/usecase 只做编排
  - facts layer 只做事务与持久化
- 目录上的 discoverability 还需要再增强，避免维护者仍旧只能从几个中心文件入手

## Recommended Adaptation for SimiClaw

适合 SimiClaw 的不是照搬 PicoClaw，而是吸收它的“能力显式化”优点，然后保留我们自己的不变量边界。

### Repo Shape

保留当前顶层仓库结构：

```text
cmd/
docs/
internal/
pkg/
tests/
web/
workspace/
```

原因：

- 这套顶层结构已经和当前文档、测试、工作区模型绑定
- 现在最乱的不是仓库顶层，而是 `internal/` 内部的能力表达

### Internal Shape

在 `internal/` 里吸收 PicoClaw 的“能力显式化”思路，但保留 SimiClaw 的边界约束：

```text
internal/
  bootstrap/
  gateway/
    model/
    bindings/
    routing/
  runtime/
    kernel/
    payload/
    workers/
    lanes/
    model/
  runner/
    context/
    tools/
    model/
  http/
    ingest/
    query/
    stream/
    middleware/
  channels/
    cli/
    telegram/
    common/
  outbound/
    delivery/
    sender/
    retry/
  provider/
  memory/
  tools/
  store/
  query/
  workspace/
  workspacefile/
  config/
```

这版结构的目标不是追求“目录多”，而是让以下能力变成显式 owner：

- gateway / routing
- runtime kernel
- worker lifecycle
- HTTP 和 channel 外部边界
- delivery policies
- memory / tools
- facts layer

这里还包含一个明确的收敛决定：

- `session` 相关的 key/scope/binding 规则先并入 `gateway/bindings/`
- 只有当 session 真正长成独立领域后，才再拆出顶层 `internal/session/`

### Use-Case Centers

真正决定后续可扩展性的，不只是目录名，而是每个能力下面有没有稳定用例中心。建议后续代码组织围绕这些一级用例：

- `ingest event`
- `claim runnable work`
- `execute event`
- `finalize run`
- `deliver outbox`
- `recover processing`
- `fire scheduled job`

这一步比单纯“拆成更多包”更重要。

## Impact on Current Speckit Plan

基于 PicoClaw 的参考，这次计划建议做如下强化：

### 1. 保留当前 Phase 顺序，不调整大施工顺序

PicoClaw 提醒我们目录 discoverability 很重要，但它不改变我们的迁移优先级。仍然应该先做：

1. runtime kernel
2. gateway/routing
3. http/channels/delivery
4. concurrency lanes

### 2. 在每个 Phase 里增加“能力 owner 显式化”目标

不仅要重构代码，还要确保每个阶段结束后，维护者能从目录直接定位该能力。

### 3. 避免“平铺一堆公共包”

不要为了模仿参考项目，把大量能力直接平铺到 `pkg/` 或根目录公共包。SimiClaw 更适合：

- 对外稳定契约放 `pkg/api`
- 共享基础类型放 `pkg/model`
- 真正的实现与能力 owner 留在 `internal/`

### 4. 把文档结构继续做成主题化索引

这一点可以继续借鉴 PicoClaw 的 `docs/` 分法。后续可以考虑把：

- channels
- routing
- delivery
- lanes
- intelligence

都补成单独的 design docs，而不是继续把这些说明堆回总文档。

## Conclusion

PicoClaw 给我们的最大启发不是某个具体技术实现，而是：

- 能力应该是显式目录
- 文档应该按主题组织
- 维护者应该能“从目录找到 owner”

但 SimiClaw 不能直接复制它的包形状。我们必须在“能力 discoverability”和“SQLite-first 运行不变量”之间取平衡。

因此，本次重构的正确方向不是“把 SimiClaw 改成 PicoClaw 那样”，而是：

**借鉴 PicoClaw 的能力化目录思路，在 `internal/` 内部重建一个更清晰的 kernel-first、facts-preserving 结构。**
