# PR: awesome-mcp-servers

**Base**: `punkpeye/awesome-mcp-servers` `main`, README blob `539d1b0e02…` (fetched 2026-10-25).
**Patch**: [`awesome-mcp-servers.patch`](awesome-mcp-servers.patch) — one added line, applies with `git apply`.
**Status**: not submitted.

## Title

```
Add memalyze — self-hosted agent memory with write-path redaction and audit
```

## Body

```markdown
Adds **memalyze**, a self-hosted memory server for agents, under *Knowledge & Memory*.

What makes it different from the other memory servers in the list: redaction runs
**before** anything is written. Phone numbers, e-mail addresses, national ID numbers,
bank cards, API keys and private keys are replaced with stable markers before dedup,
FTS indexing and embedding, so the plaintext never reaches the database. Every
`memory_add` / `memory_search` / `memory_delete` is appended to an audit table
(query stored as SHA-256 + length, exportable as CSV), and the table rejects UPDATE
at the database level.

- Go, one static binary (~14 MB), no Python/Node runtime
- MCP over stdio or Streamable HTTP; four tools total
- SQLite storage, local embeddings via Ollama (`qwen3-embedding:0.6b`)
- If Ollama is down, search degrades to keyword mode and reports `degraded=true`
  instead of hanging
- AGPL-3.0, with a commercial license available

~~~sh
curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh
ollama pull qwen3-embedding:0.6b
memalyze demo   # 30 s, no Ollama and no network needed
~~~

Repo: https://github.com/mahaoboy/memalyze

Prebuilt binaries for Linux, macOS and Windows are on the v0.1.0 release page.
Limits are written down rather than implied away:
one token per process (no per-user auth), pattern-based redaction (not a compliance
certificate), no audit UI — see SECURITY.md.

Added the Glama score badge — the listing is at
https://glama.ai/mcp/servers/mahaoboy/memalyze. I appended the entry at the end of the
section to keep the diff to one line — say the word if you would rather have it alphabetised.
```

## How to submit

```sh
# 1. fork on GitHub, then:
git clone https://github.com/<you>/awesome-mcp-servers.git
cd awesome-mcp-servers
git checkout -b add-memalyze
git apply /path/to/memalyze/docs/submissions/awesome-mcp-servers.patch
git add README.md
git commit -m "Add memalyze (self-hosted memory with redaction + audit)"
git push origin add-memalyze
```

Open the PR against `punkpeye/awesome-mcp-servers:main` with the title above.

Notes:
- The repo's CONTRIBUTING asks for alphabetical order, but the section is not sorted
  in practice; the patch appends. Move the line if a reviewer asks.
- If the PR is opened by an automated agent, their CONTRIBUTING says to append
  `🤖🤖🤖` to the title for fast-tracked review. Do not add it to a human PR.
- Re-fetch the README before applying if `main` has moved: the patch has 3 lines of
  context and will fail loudly rather than corrupt the file.
