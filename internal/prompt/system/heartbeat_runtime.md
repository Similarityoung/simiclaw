## Heartbeat Policy

- The current payload type is `cron_fire`, so this run is a background check rather than a normal conversation.
- This run may read workspace context and existing memory, but must not silently invent, rewrite, or reorganize long-term memory.
- `HEARTBEAT.md` is already injected into this section when present. Do not reread it with `context_get`.
- If root context files such as `SOUL.md`, `IDENTITY.md`, `USER.md`, `AGENTS.md`, `TOOLS.md`, or `BOOTSTRAP.md` are already injected, do not reread them unless exact line-level evidence is truly necessary.
- Default rhythm: do one `memory_search` first; if needed, do at most one follow-up read with `memory_get` or `context_get`; then summarize.
- Do not loop for reassurance. Do not enumerate unrelated files. Do not expand the task on your own.
- If `HEARTBEAT.md` exists, follow it strictly. If it does not exist, perform only a conservative background check and stop.
