# Directory submissions — ready-to-paste

Everything here is prepared so submission is copy/paste + one PR, not writing.
Update this file after each submission with the date and link.

## 1. punkpeye/awesome-mcp-servers

Fork → edit `README.md` in the **Databases** (or **Memory**) section → add one line:

```markdown
- [mahaoboy/memalyze](https://github.com/mahaoboy/memalyze) 📇 🏠 🐧 🍎 🪟 - Self-hosted memory layer for agents: redaction before write, append-only audit, local embeddings, one static binary.
```

Legend used by that repo: `📇` = has a `server.json`/manifest, `🏠` = local/self-hosted,
`🐧🍎🪟` = OS support. Drop the OS icons if any target is missing.

PR title: `Add memalyze (self-hosted memory with redaction + audit)`
PR body: one paragraph from `README.md` §"The problem", plus the three-step install.

## 2. modelcontextprotocol/servers (community list)

That repo only accepts a README entry pointing at an external server implementation:

```markdown
- **[memalyze](https://github.com/mahaoboy/memalyze)** - Self-hosted memory for agents with write-path redaction and append-only audit (Go, single binary).
```

Read their `CONTRIBUTING.md` before opening: they require the server to be installable
without cloning (ours is: release binaries + `curl | sh`).

## 3. Official MCP registry (`registry.modelcontextprotocol.io`)

The registry wants a `server.json` **and** a distributable package. Our binary is not on
npm/PyPI, so the realistic options are:

| Option | Requires | Status |
|---|---|---|
| `registryType: oci` | publish `ghcr.io/mahaoboy/memalyze:v0.1.0` | ⬜ blocked on Docker image build (B5) |
| `registryType: mcpb` | bundle the binary + manifest into `.mcpb` | ⬜ not attempted |

Draft `server.json` (fill after the image exists):

```json
{
  "$schema": "https://static.modelcontextprotocol.io/schemas/2025-09-29/server.schema.json",
  "name": "io.github.mahaoboy/memalyze",
  "description": "Self-hosted memory for agents: redaction before write, append-only audit, local embeddings.",
  "repository": { "url": "https://github.com/mahaoboy/memalyze", "source": "github" },
  "version": "0.1.0",
  "packages": [
    {
      "registryType": "oci",
      "identifier": "ghcr.io/mahaoboy/memalyze",
      "version": "0.1.0",
      "transport": { "type": "stdio" }
    }
  ]
}
```

Do **not** submit a `server.json` pointing at an image that does not exist.

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
| awesome-mcp-servers | — | — | — |
| modelcontextprotocol/servers | — | — | — |
| MCP registry | — | — | blocked on image |
| ClawHub | — | — | — |
