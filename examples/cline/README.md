# Cline + memalyze

Cline stores MCP servers in `cline_mcp_settings.json`. The on-disk location has
changed between versions, so use the UI button instead of hunting for it:

1. Open the **MCP Servers** panel.
2. Open the **Configure** tab.
3. Click **Configure MCP Servers** — this opens the right JSON file.
4. Merge the `memalyze` entry from [`cline_mcp_settings.json`](cline_mcp_settings.json).
5. Save; Cline reloads the list. If not, use **Restart Server**.
6. Expand the entry and confirm `echo`, `memory_add`, `memory_search`,
   `memory_delete`.

Replace `/absolute/path/to/memalyze` with the absolute path to the binary. Use an
absolute path for `MEMALYZE_DB` too; GUI-launched editors do not always inherit your
shell's working directory or `PATH`.

## Rules

Copy [`.clinerules`](.clinerules) to the project root, or paste its contents into
Cline's **Custom Instructions** (global or workspace, depending on how you scope
memory). Without rules the model sees the tools but may answer from its own memory.

## Approval

The example auto-approves only `memory_search`:

```json
"autoApprove": ["memory_search"]
```

Do **not** auto-approve `memory_add` or `memory_delete`. Writes and deletes should
remain visible to the user; `memory_add` is only called when the user asks to
remember something.

## Cline CLI (headless)

Cline CLI 3.x uses different config paths from the IDE. With `--data-dir`, the agent
loads MCP servers from:

```text
$DATA_DIR/settings/cline_mcp_settings.json
```

`cline config mcp --config <dir>` reads:

```text
<dir>/data/settings/cline_mcp_settings.json
```

If you use both flags, keep both files in sync (the S14 test did). For the IDE, use
the **Configure MCP Servers** button and ignore this section.

Tool names are namespaced by the harness: Cline exposes `memalyze__memory_search`,
`memalyze__memory_add`, etc. The prompt can keep saying `memory_search`; the model
maps it to the exposed name. Do not hard-code the namespaced name in a portable
prompt.

Cline CLI 3.0.73 also rejects a pure-Chinese prompt that contains no whitespace
(`Unknown command or unquoted prompt`). Add a space:

```bash
cline --json --provider openai-compatible --model deepseek-v4-flash \
  --config /path/to/cline-config --data-dir /path/to/cline-data \
  --auto-approve true "请记住 周日强制休息"
```

This is a CLI parser quirk, not a memalyze issue; the Cline IDE does not have it.

## Verify

Ask Cline:

```text
我上次说的休息日是哪天？
```

Expected: Cline calls `memory_search`, then answers from `hits[].content` (for
example “周日强制休息”). The first call may be slower while the embedding model
loads; memalyze warms it in the background and keeps it resident for
`MEMALYZE_KEEP_ALIVE` (default `30m`). A query that arrives during the warmup window
can still return `degraded=true`; keyword fallback works, but semantic recall is
temporarily unavailable.

## Older path

On older VS Code installs the file lived at:

```text
~/.config/Code/User/globalStorage/saoudrizwan.claude-dev/settings/cline_mcp_settings.json
```

If that file exists, you can edit it directly. The UI button remains the reliable
route after upgrades.
