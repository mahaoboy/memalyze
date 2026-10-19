# Claude Code + memalyze

This example uses **project-scope** MCP config (`.mcp.json` is committed with the
project) plus a `CLAUDE.md` rule that makes the model search memory before it answers
personal questions.

## 1. Build and register

```bash
cd /path/to/memalyze
go build -o memalyze ./cmd/memalyze

# Run this from the project where you want Claude Code to use memalyze.
claude mcp add --scope project memalyze -- /path/to/memalyze
```

That command writes `.mcp.json` in the current directory. The generated file looks
like [`/.mcp.json`](.mcp.json) — edit the absolute paths and the optional
`MEMALYZE_DB` / `MEMALYZE_TENANT` values.

Alternatively, copy [`.mcp.json`](.mcp.json) to your project root and replace the
placeholders.

## 2. Add the rules

Copy [`CLAUDE.md`](CLAUDE.md) to the project root, or merge its block into an
existing `CLAUDE.md`. Without this step Claude Code sees the tools but is free to
answer from its own memory — and in testing a small model did exactly that, including
guessing a date.

## 3. Approve and verify

```bash
claude mcp get memalyze      # should show the project scope and command
```

Open the project in Claude Code and run `/mcp`. Project-scope servers start as
**pending approval**; approve `memalyze`, then check that `echo`, `memory_add`,
`memory_search`, and `memory_delete` are listed.

Ask:

```text
我上次说的休息日是哪天？
```

Expected: the transcript shows a `memory_search` call before the answer, and the
answer uses the returned `hits[].content` (for example “周日强制休息”).

## Notes

- Use an absolute path for `command`: Claude Code may not inherit your shell `PATH`.
- `MEMALYZE_DB` defaults to `./memalyze.db` relative to the server process. Set it
  explicitly to share one database across all projects.
- Never commit a real database file. Add `*.db` to `.gitignore`.
- The model call itself was not verified on the development machine because Claude
  Code was not logged in (`Not logged in · Please run /login`); the MCP config
  format was validated with `claude mcp add/get`.
