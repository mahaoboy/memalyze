# S25 — DeepSeek Harness plugin package

- **Status**: MVP verified end-to-end on Linux x64 (2026-11-17); published to npm as
  `dsh-plugin-memalyze@0.2.0` and installed from the registry (2026-11-18); macOS/Windows
  binaries built but not executed on real hardware; dsh-plugin.org submission still pending
- **Milestone**: M5 / distribution
- **Scope**: `integrations/dsh/` (Cordis setup plugin + patch + runtime skill + tests),
  `scripts/build-dsh-plugin.sh`, 5-platform gzipped binaries, one npm tarball.

## 1. Why a plugin instead of "install the binary yourself"

A DSH user should get memory with `dsh plugin --profile web add dsh-plugin-memalyze` and a
restart. Three problems had to be solved before that sentence is true — the same three that
were ranked by risk:

1. **Binary delivery.** DSH installs run through pnpm, which blocks npm `postinstall`
   scripts. A package that downloads a binary at install time would silently do nothing.
2. **Fixed database path.** A bare `./memalyze.db` lands in whatever directory DSH was
   started from, and first-run SQLite creates no parent directory.
3. **No-Ollama users.** Embeddings are optional by design, but the plugin must still add
   and search memories when Ollama is absent, instead of failing at startup.

## 2. Design

### 2.1 Binary delivery (no install script)

- `scripts/build-dsh-plugin.sh` cross-compiles five static binaries
  (`CGO_ENABLED=0`, `-trimpath`, version injected) and gzips them into
  `integrations/dsh/bin/`:
  `linux-amd64`, `linux-arm64`, `darwin-amd64`, `darwin-arm64`, `windows-amd64.exe`.
- The Cordis setup plugin (`index.js`) runs **at profile boot**, synchronously copies the
  asset for `process.platform`/`process.arch` into `~/.memalyze/bin/` (mode `0755`,
  directory `0700`), and writes `.installed-version` so a plugin upgrade replaces the binary
  and a restart with the same version does not. A failed replace restores the backup.
- The MCP bridge is configured with computed absolute paths (no `${HOME}` templating, whose
  empty interpolation had already bitten us):

```yaml
command: <homedir>/.memalyze/bin/memalyze[.exe]
env:
  MEMALYZE_DB: <homedir>/.memalyze/memalyze.db
  MEMALYZE_ACTOR: dsh
cwd: <homedir>/.memalyze
```

- Result: the database and binary are shared by every DSH workspace on the machine, and
  nothing depends on shell `PATH` (which a GUI-launched DSH may not have).

### 2.2 Database directory

`cmd/memalyze/main.go` now calls `ensureDBDir` before `sqlite.Open`: the parent of a
non-memory, non-`file:` path is created with `0700`. Covered by
`TestEnsureDBDirCreatesParent` / `TestEnsureDBDirSkipsNonFiles`.

### 2.3 Skill registration

The plugin's `skills/` directory is **not** auto-discovered by DSH, so `index.js` reads
`SKILL.md`, parses its frontmatter and registers it through `ctx.skills.register` once the
`skills` service appears (retried up to 40 × 250 ms). The registration carries
`source: 'runtime'` and `provider: 'memalyze'`.

### 2.4 Package layout

```
integrations/dsh/
  package.json          dsh.bundle.patch -> ./cordis.patch.yml
  cordis.patch.yml      memalyze-setup + @deepseek-ai/dsh-mcp-client
  index.js              copy binary, register skill
  SKILL.md              agent-facing rules (tool names are mcp__memalyze__*)
  README.md             user install / verify / uninstall / troubleshooting
  bin/*.gz              5 binaries (built, gitignored)
  test/setup-plugin.test.mjs
  test/mcp-smoke.mjs
```

### 2.5 One contract, many harnesses

The DSH plugin does not fork the product. The Go binary, MCP server, SQLite schema and the
four tools are shared with the Claude Code, Cline, MCPB and HTTP paths; the DSH layer only
chooses an install location and a tool prefix (`mcp__memalyze__*`).

To stop the agent instructions from drifting, `docs/agent-contract.md` is the single source
for "when to search / scope / query / result semantics". `scripts/gen-agent-contracts.mjs`
renders it into five files between markers:

- `examples/claude-code/CLAUDE.md` (bare tool names)
- `examples/cline/.clinerules` (bare)
- `examples/custom-harness/system-prompt.md` (bare)
- `integrations/dsh/SKILL.md` (`mcp__memalyze__*`)
- `skills/memalyze/SKILL.md` (bare)

`make contracts` rewrites them; `make contracts-check` exits non-zero on drift and runs in
CI. The CI job `harness-compat (stdio)` also builds the host binary and runs
`integrations/dsh/test/mcp-smoke.mjs` with `MEMALYZE_BIN`, i.e. raw JSON-RPC with no DSH
and no Ollama — the compatibility test for any harness.

## 3. How it was verified

Environment: Linux x64, DSH `0.2.0-rc.2`, Node `24.22.1`, pnpm `11.22.0` (hoisted),
Go toolchain of the repo. Test profile `dshmemtest` created from the default `headless`
profile; `HOME=/tmp/memalyze-dsh-home` isolated `~/.memalyze` while `DSH_HOME` kept the
real profile/credentials.

```bash
make dsh-package          # 5 binaries + npm tarball
make dsh-test             # setup plugin test + raw MCP smoke

dsh --profile dshmemtest --from-default-profile headless --dump-config
dsh plugin --profile dshmemtest add ./dist/dsh-plugin-memalyze-0.2.0.tgz
HOME=/tmp/memalyze-dsh-home DSH_HOME=/home/mahao/.dsh dsh --profile dshmemtest "..."
```

### 3.1 Build

- 5 platforms compiled; tarball `dist/dsh-plugin-memalyze-0.2.0.tgz`, **32.0 MB**
  (31 MB on disk, 11 files, sha256 `73b2a2ba…`). The size is
  the five gzipped binaries; npm's tarball limit is 500 MB.

### 3.2 Setup plugin test

- Binary copied and executable, 16,322,720 bytes (> 1 MB).
- `.installed-version` equals `0.2.0`.
- Exactly one skill registration, `source === 'runtime'`, `provider === 'memalyze'`;
  content contains `mcp__memalyze__memory_search` and the "Search first" rule; no leading
  frontmatter.

### 3.3 Raw MCP smoke (binary only, no DSH)

- `initialize`, `tools/list` → `echo, memory_add, memory_delete, memory_search`.
- Add → `embedded=false`, `embedding_failed=true`; duplicate detected; demo phone stored as
  `[REDACTED:phone]`; search → `degraded=true` in 3 ms with a keyword hit; delete; offline
  `audit export --format json` contains add/search rows.

### 3.4 Real DSH end-to-end

- Boot log: `[memalyze] binary ready: …`, `[memalyze] skill registered: memalyze`.
- Session A added `The launch code is ORANGE-42.` (scope `project:dsh-e2e`).
- Session B called the skill tool, got the full `<skill_content name="memalyze">`, then
  `memory_search` with **no scope** found the hit (`matched_by: vector+keyword`) and
  answered `ORANGE-42`.
- Uninstall + reinstall kept `~/.memalyze/memalyze.db`; the agent still recalled ORANGE-42.

### 3.5 Failure found and fixed: `source must be a string`

The first registration shipped without `source`, and the skill tool failed with
`Error: loaded skill "memalyze" source must be a string`. Fix: set `source: 'runtime'` and
`provider: 'memalyze'`; assert both in `test/setup-plugin.test.mjs`; repack and reinstall.
Re-verified as in §3.4.

### 3.6 No-Ollama through DSH

Boot with `MEMALYZE_OLLAMA_URL=http://127.0.0.1:9` (connection refused):

- warmup failed in 1 ms, server stayed up;
- `memory_add` → `embedding_failed=true`, memory saved;
- `memory_search` → `degraded=true`, `degrade_reason=vector_error`, keyword hit found;
- agent answered `embedding_failed=true, degraded=true`.

### 3.7 Contract regeneration

`make contracts-check` reports all 5 files up to date. After repacking and reinstalling in
`dshmemtest`, the skill tool returned the regenerated block (`The four rules`,
`mcp__memalyze__memory_search`) and `mcp__memalyze__echo` replied `CONTRACT-OK`.

### 3.8 npm registry end-to-end (2026-11-18)

`dsh-plugin-memalyze@0.2.0` was published from a maintainer terminal (the agent session is
policy-blocked from publishing). `npm publish` reported success, but the registry served only
`0.0.0-stage` ("Temporary package placeholder for staged publishing") for about 15 minutes
while npm scanned the version: `latest` moved to `0.2.0` at 21:36 local, and the tarball
answered 200 at 21:39. The served tarball is byte-identical to the one `make dsh-package`
produced:

- `sha256 73b2a2ba…` for both `dist/dsh-plugin-memalyze-0.2.0.tgz` and the download from
  `registry.npmjs.org` (32,006,519 bytes);
- `dist.integrity = sha512-dkWhB9Bji…` matches the publish output.

Clean-profile install from the registry (the machine's default npm registry is the
read-only `npmmirror` mirror, so pnpm needs an explicit `--registry`):

```bash
dsh --profile dshnpmtest --from-default-profile headless --dump-config
dsh plugin --profile dshnpmtest add dsh-plugin-memalyze@0.2.0 \
    --registry=https://registry.npmjs.org/
HOME=/tmp/memalyze-dsh-home-npm DSH_HOME=/home/mahao/.dsh \
    dsh --profile dshnpmtest "..."
```

Result:

- pnpm resolved and installed `0.2.0` from npmjs.org in 5 s; plain
  `npm_config_registry=…` is ignored by the DSH/pnpm path, `--registry` is not.
- boot logged `[memalyze] binary ready: /tmp/memalyze-dsh-home-npm/.memalyze/bin/memalyze`
  and `[memalyze] skill registered: memalyze`.
- session A stored `The npm launch code is BLUE-7` under `project:npme2e`
  (`id=01M7QE7MCAQ740FQR76KB7CJTT`; embedding warmup 21.5 s on a cold model).
- a second `dsh` process with the same isolated `HOME` searched with **no scope** and
  answered `BLUE-7`, so the registry-installed package, binary, skill and tool prefix all
  work end-to-end.

A version can be published only once: do not retry `publish` while the placeholder is
showing, and treat `0.0.0-stage` as a scan artifact rather than a usable release.

## 4. Known gaps

| Gap | Impact | Next |
|---|---|---|
| macOS arm64/x64 and Windows x64 binaries not executed | first-user risk on non-Linux | run `make dsh-test` on those machines before announcing |
| npm serves `0.0.0-stage` before a scanned version | first install after publish can pick the placeholder | wait for `npm view … version` to equal the release; never republish the same version |
| Not submitted to dsh-plugin.org | needs public repo + `dsh-plugin` GitHub topic + tarball | do after publish decision |
| MCP `toolCallTimeoutMs` is 30 s | a cold Ollama model load (>20 s) plus search could exceed it | acceptable for now; 3 s query budget already caps search |
| No automatic upgrade of `~/.memalyze/bin` while the old binary is running on Windows | marker version changes only after a restart | documented as "restart DSH after upgrading" |

## 5. Commands for the next round

```bash
make dsh-package && make dsh-test
cd integrations/dsh && npm publish        # only after approval
# dsh-plugin.org: ensure repo is public, add GitHub topic `dsh-plugin`, submit the tgz
```
