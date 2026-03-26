# Prompt And Workspace Context

## Summary

SimiClaw 把 prompt 视为一条有固定层次的 system message，而不是一堆随意拼接的文本。workspace 文件、curated memory、skills 索引和当前 run context 都有明确职责与注入顺序。

## Context

相关代码主要在 `internal/prompt/`（含 `system/*.md` 模板）、`internal/memory/`、`internal/workspace/` 和 `internal/workspacefile/`。这一层负责“给模型看什么”和“模型能安全改什么”。`internal/tools/` 属于 Capability Plane；prompt 只暴露工具契约文本或 skills 索引，不直接拥有 tool invocation。

## Details

### Prompt 固定结构

`promptRenderer.renderStatic` 的 section 顺序是固定的：

1. `Identity & Runtime Rules`
2. `Tool Contract`
3. `Memory Policy`
4. `Workspace Instructions & Context`
5. `Available Skills`
6. `Heartbeat Policy`（仅 `payload.type=cron_fire`）
7. `Current Run Context`

### Workspace 上下文文件

普通 run 会按以下顺序读取 workspace 根目录上下文文件，存在才注入：

1. `SOUL.md`
2. `IDENTITY.md`
3. `USER.md`
4. `AGENTS.md`
5. `TOOLS.md`
6. `BOOTSTRAP.md`

`HEARTBEAT.md` 只在 `cron_fire` 变体中注入。`init` 会自动 scaffold 缺失的模板文件，但不会自动创建 `AGENTS.md`，也不会覆盖已有文件。

### Skills

- skills 位于 `workspace/skills/<name>/SKILL.md`
- prompt 只注入紧凑 skill 索引：`name / description / path`
- skill 正文不会自动塞进 prompt；需要正文时应通过 `context_get` 读取
- tool / provider 的真实执行发生在 Runtime <-> Capability Plane 边界，不在 prompt/context owner 内直接推进

### Memory

- memory 的维度:
  - `visibility`: `public | private`
  - `kind`: `curated | daily`
- curated memory 的 canonical 路径:
  - `memory/public/MEMORY.md`
  - `memory/private/MEMORY.md`
- daily memory 的 canonical 路径:
  - `memory/public/daily/YYYY-MM-DD.md`
  - `memory/private/daily/YYYY-MM-DD.md`
- prompt 默认只注入 curated memory；`private` curated 仅在 `dm` channel 注入
- `memory_flush` / `compaction` 可能把内容写回 memory 文件；daily memory 默认通过 `memory_search` / `memory_get` recall，而不是直接注入

### Workspace 文件边界

- `context_get` 只能读取固定上下文文件或 skill 正文
- `workspace_patch` / `workspace_delete` 只能在 workspace 内操作 UTF-8 文本
- `workspacefile.ResolveContextPath` 只允许根上下文文件与 `skills/<name>/SKILL.md`
- `workspacefile.ResolvePath` 会拒绝越界路径、`runtime/` 路径，以及非 `dm` 场景下的 private memory 路径
- 创建、patch、delete 与 context read 都依赖同一 owner 下的路径解析和文本校验逻辑

## Verification

- prompt 组装与渲染: `internal/prompt/builder.go`, `internal/prompt/loader.go`, `internal/prompt/renderer.go`, `internal/prompt/system_text.go`
- scaffold 行为: `internal/workspace/scaffold.go`
- memory 写入与检索: `internal/memory/writer.go`, `internal/memory/search.go`
- 工具暴露: `internal/tools/context_get.go`, `internal/tools/workspace_file.go`
- 文件安全边界: `internal/workspacefile/workspacefile.go`, `internal/workspacefile/context.go`

## Related Docs

- 系统总览: [`../../ARCHITECTURE.md`](../../ARCHITECTURE.md)
- 运行链路: [`runtime-flow.md`](runtime-flow.md)
- 工作区布局: [`../references/workspace-layout.md`](../references/workspace-layout.md)
- 配置参考: [`../references/configuration.md`](../references/configuration.md)
