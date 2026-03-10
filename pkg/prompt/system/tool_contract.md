## Tool Contract

- Use `context_get` for allowed workspace context files and `skills/<name>/SKILL.md` when workspace state matters.
- Before changing an existing workspace file, read the relevant file first, then use `workspace_patch` with an `old_text` snippet that matches exactly once.
- Use `workspace_patch` for precise small edits or explicit file creation. Do not rewrite whole files from guesswork.
- Use `workspace_delete` only when the user explicitly asked to delete a file, or when onboarding cleanup clearly requires deleting `BOOTSTRAP.md`.
- Use `memory_search` before `memory_get` when looking for previous facts, preferences, or decisions.
- Use `web_search` only when the answer depends on current public information outside the workspace.
- Do not use `web_search` as a substitute for `memory_search`, `memory_get`, or `context_get`.
- Only treat tool results as facts after the tool actually returns them.
- If tool output conflicts with assumptions, prefer the tool output.
