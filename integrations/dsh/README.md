# dsh-plugin-memalyze

Persistent, self-hosted memory for [DeepSeek Harness](https://github.com/deepseek-ai/deepseek-harness), packaged as a DSH profile bundle.

One static Go binary, one SQLite file, four MCP tools. Memories survive sessions
and workspaces. Redaction runs before a write; every search is audited.

## Install

```bash
dsh plugin --profile web add dsh-plugin-memalyze
```

Restart DSH. The plugin installs the matching binary and creates the database on
first boot:

```text
~/.memalyze/bin/memalyze        # executable (memalyze.exe on Windows)
~/.memalyze/memalyze.db         # SQLite database
```

Use your own profile name instead of `web` if you boot a different one
(`headless`, `tui`, a custom profile, ...). The bundle must be enabled; `dsh
plugin add` does that automatically for packages that declare `dsh.bundle`.

### From a local tarball

```bash
npm pack                       # in this directory; produces dsh-plugin-memalyze-0.2.0.tgz
dsh plugin --profile web add ./dsh-plugin-memalyze-0.2.0.tgz
```

A source checkout must run `make dsh-package` first, because the binary assets
are build artifacts and are not committed.

## Requirements

- DSH 0.2.0-rc.2 or newer
- Node.js 22.19 or newer (DSH's own minimum)
- Linux x64/arm64, macOS x64/arm64, or Windows x64

**Ollama is optional.** Without it, `memory_add` still stores the memory with
`embedding_failed: true` and `memory_search` falls back to keyword/FTS within its
3-second budget (`degraded: true`). Install Ollama when you want semantic recall:

```bash
ollama pull qwen3-embedding:0.6b
```

## What the model sees

The official stdio MCP bridge exposes:

| Tool | Purpose |
|---|---|
| `mcp__memalyze__memory_add` | store a memory (explicit user request only) |
| `mcp__memalyze__memory_search` | retrieve by meaning or keyword |
| `mcp__memalyze__memory_delete` | delete one memory by id |
| `mcp__memalyze__echo` | prove the MCP link works |

The plugin also registers the `memalyze` skill, which tells the model **when** to
search and **how** to read `duplicate`, `embedding_failed`, `degraded`, and
`redacted` results. Prompting is the lever: a soft hint is ignored, a MUST-level
search-before-answering contract is followed.

## Verify

1. Ask DSH: "Remember that the launch code is ORANGE-42."
2. Start a new session (or switch workspace) and ask: "What did I ask you to
   remember about the launch code?"
3. The answer should come from `mcp__memalyze__memory_search`, not from the
   current conversation.

A direct wiring check:

```bash
~/.memalyze/bin/memalyze --version
~/.memalyze/bin/memalyze memory list --db ~/.memalyze/memalyze.db
```

## Governance

- **Redaction before write.** Phone numbers, e-mail, resident ID, bank card,
  API keys and private keys are replaced before the row is stored (`enforce`
  default). `MEMALYZE_REDACT_MODE=off` disables it; `dry-run` records what would
  have been redacted.
- **Append-only audit.** Every add, search and delete is recorded with a
  SHA-256 hash chain. Query text is hashed by default.

```bash
~/.memalyze/bin/memalyze audit export --db ~/.memalyze/memalyze.db --format csv
~/.memalyze/bin/memalyze audit verify --db ~/.memalyze/memalyze.db
```

- **Local by default.** stdio only; no cloud, no account. The database is
  readable by the current user (`~/.memalyze` is created with mode 0700).

Configuration is environment-based. The bundle sets only `MEMALYZE_DB` and
`MEMALYZE_ACTOR=dsh`; ambient `MEMALYZE_*` variables still apply. See the
[main README](https://github.com/mahaoboy/memalyze#configuration) for the full
table.

## Uninstall

```bash
dsh plugin --profile web remove dsh-plugin-memalyze
```

The plugin removes its profile layer, not your data. Delete `~/.memalyze` only
when you also want to delete every memory and the audit log.

## Development

```bash
make dsh-package          # build 5 platform binaries, gzip, npm pack
node test/setup-plugin.test.mjs
node test/mcp-smoke.mjs
```

The package intentionally has no install script. pnpm 10 blocks dependency build
scripts unless allowlisted, and a `postinstall` copy would also run before the
user's home is known. The Cordis setup plugin copies the binary synchronously
instead.

## License

AGPL-3.0-only. The memalyze source and release binaries are at
[mahaoboy/memalyze](https://github.com/mahaoboy/memalyze).
