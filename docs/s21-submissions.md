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
- **Requires**: Ollama + `qwen3-embedding:0.6b` (or any OpenAI-compatible embedder later)
- **Homepage**: https://github.com/mahaoboy/memalyze

## 5. Tracking

| Directory | Submitted | Link | Outcome |
|---|---|---|---|
| awesome-mcp-servers | — | — | patch ready (`submissions/awesome-mcp-servers.patch`) |
| modelcontextprotocol/servers | n/a | — | not accepted (issues only) |
| MCP registry | — | — | assets uploaded 2026-10-25; metadata ready; remaining = `mcp-publisher login github` + publish |
| ClawHub | — | — | skill bundle ready |
