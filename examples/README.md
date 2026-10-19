# Integration examples

S13 is about **making the model use `memory_search`**, not adding tools.

The pattern is always the same:

1. Connect memalyze over MCP (stdio).
2. Pass the four tool schemas to the model.
3. Install the system prompt / project rules below, so the model searches before
   answering questions about the user's past.
4. Inject only `hits[].content` into the next turn — never raw JSON, never all
   memories on every turn.
5. Keep writes explicit: `memory_add` only when the user asks to remember.

| Harness | What to copy | Notes |
|---|---|---|
| Claude Code | [`claude-code/CLAUDE.md`](claude-code/CLAUDE.md) + [`claude-code/.mcp.json`](claude-code/.mcp.json) | `.mcp.json` is project scope; approve the server in `/mcp` |
| Cline | [`cline/.clinerules`](cline/.clinerules) + [`cline/cline_mcp_settings.json`](cline/cline_mcp_settings.json) | Only auto-approve `memory_search` |
| Custom harness | [`custom-harness/system-prompt.md`](custom-harness/system-prompt.md) + [`custom-harness/README.md`](custom-harness/README.md) | You own the tool loop and context budget |

Full rationale, tested prompt variants, and verification output:
[`docs/s13-context-injection.md`](../docs/s13-context-injection.md).
