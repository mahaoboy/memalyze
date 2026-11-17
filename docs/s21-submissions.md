# Directory submissions — ready-to-paste

Everything here is prepared so submission is copy/paste + one PR, not writing.
Update this file after each submission with the date and link.

## 1. punkpeye/awesome-mcp-servers

Fork → edit `README.md` → **Knowledge & Memory** section → append one line:

```markdown
- [mahaoboy/memalyze](https://github.com/mahaoboy/memalyze) 🏎️ 🏠 🍎 🪟 🐧 - Self-hosted agent memory in one Go binary: redacts phone numbers, e-mail, ID cards, bank cards and API keys before they are written, audits every search (CSV export, query stored as SHA-256), local Ollama embeddings, four MCP tools, no cloud.
```

Legend (from that repo): `🏎️` = Go, `🏠` = local/self-hosted, `🍎🪟🐧` = macOS/Windows/Linux.
(The old draft used `📇`, which means TypeScript — wrong language icon.)

Do not hand-edit: apply
[`submissions/awesome-mcp-servers.patch`](submissions/awesome-mcp-servers.patch), which was
checked with `git apply --check` against README blob `539d1b0e02…` (2026-10-25). Title, body
and the exact commands are in
[`submissions/awesome-mcp-servers-pr.md`](submissions/awesome-mcp-servers-pr.md).

## 2. modelcontextprotocol/servers — do not submit

Obsolete as of 2026-10-25. Their `CONTRIBUTING.md` says **"We accept issues, not pull
requests"**, and the README now keeps only the official reference servers and points
directory traffic at the registry. Opening a PR there would be closed automatically.
If something is genuinely broken, open an **issue**.

## 3. Official MCP registry (`registry.modelcontextprotocol.io`)

The registry stores metadata only; the artifact lives on GitHub Releases. **MCPB is an
official package type**, so this path does not need the Docker image at all (it used to be
blocked on B5; see D-023).

- Ready-to-publish metadata: [`../deploy/registry/server.json`](../deploy/registry/server.json)
- Full runbook (upload → validate → login → publish): [`submissions/README.md`](submissions/README.md)
- Bundles: `dist/memalyze-<version>-<os>-<arch>.mcpb` (built by `scripts/build-mcpb.sh`)

The `fileSha256` values are generated from the files, never typed by hand. Rebuild the
bundles and the file must be regenerated in the same run:

```sh
make release            # 5 platforms, tagged binary version
make mcpb               # 4 bundles + dist/MCPB-SHA256SUMS + deploy/registry/server.json
```

> Do **not** publish a `server.json` whose package URLs 404. Upload the `.mcpb` assets to
> the release first.

## 4. ClawHub / dsh skill catalog

The skill lives at `skills/memalyze/` (`SKILL.md` + `README.md`). Package it:

```bash
make skill          # dist/memalyze-skill-<version>.tar.gz
```

Submission fields (adapt to whatever the form asks):

- **Name**: `memalyze`
- **One-liner**: persistent, self-hosted memory for agents — redaction before write, audit on read
- **Install**: `curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh`
- **Requires**: Ollama optional — without it adds still succeed and search uses keyword/FTS (`degraded: true`)
- **Homepage**: https://github.com/mahaoboy/memalyze

## 5. DeepSeek Harness plugin (`dsh-plugin-memalyze`)

One-command install inside DSH; verified end-to-end (see [`s25-dsh-plugin.md`](s25-dsh-plugin.md)).

```bash
dsh plugin --profile web add dsh-plugin-memalyze   # or ./dist/dsh-plugin-memalyze-0.2.0.tgz
# restart DSH
```

- Package: `integrations/dsh/` → `dist/dsh-plugin-memalyze-<version>.tgz`
  (5 gzipped binaries; 32 MB; npm tarball limit is 500 MB)
- Rebuild: `make dsh-package && make dsh-test`
- No install script: the Cordis setup plugin copies the binary to `~/.memalyze/bin` at boot
- dsh-plugin.org prerequisites: **public** GitHub repo, `dsh-plugin` topic on the repo,
  tarball URL. Neither the topic nor the submission has been done yet.

DSH plugin listing fields:

- **Name**: `dsh-plugin-memalyze`
- **One-liner**: persistent, self-hosted memory for DSH agents — redaction before write, audit on read
- **Install**: `dsh plugin --profile <profile> add dsh-plugin-memalyze`
- **Requires**: DSH 0.2.0-rc.2+, Node ≥ 22.19; Ollama optional
- **Repo**: https://github.com/mahaoboy/memalyze/tree/main/integrations/dsh

## 6. Claude marketplace

Claude's directory rules were checked earlier: ≤ 256 KiB per file, repo < 50 MiB, no
compiled executables — so the 32 MB binary tarball cannot go there. The route is a
**self-hosted marketplace** (`.claude-plugin/marketplace.json` + `plugins/memalyze/` that
references the released binary), which Claude Code can add from a Git URL. Skeleton not
written yet.

## 7. Tracking

| Directory | Submitted | Link | Outcome |
|---|---|---|---|
| awesome-mcp-servers | 2026-10-25 | https://github.com/punkpeye/awesome-mcp-servers/pull/17582 | 🟡 open, waiting for review |
| modelcontextprotocol/servers | n/a | — | not accepted (issues only) |
| MCP registry | 2026-10-25 | https://registry.modelcontextprotocol.io/v0/servers?search=memalyze | ✅ published `io.github.mahaoboy/memalyze@0.1.0` (4 mcpb packages) |
| ClawHub | — | — | skill bundle ready |
| DSH plugin (`dsh-plugin-memalyze`) | — | local tgz, sha256 `26e0d421…` | package + e2e verified; npm publish pending approval |
| dsh-plugin.org | — | — | blocked on public repo + `dsh-plugin` topic |
| GitHub topic `dsh-plugin` | — | — | not added |
| Claude self-hosted marketplace | — | — | skeleton not written |
