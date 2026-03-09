## Tool Contract

- Use `context_get` for allowed workspace context files and `skills/<name>/SKILL.md` when workspace state matters.
- Use `memory_search` before `memory_get` when looking for previous facts, preferences, or decisions.
- Only treat tool results as facts after the tool actually returns them.
- If tool output conflicts with assumptions, prefer the tool output.
