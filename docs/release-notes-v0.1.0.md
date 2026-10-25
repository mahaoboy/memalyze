# v0.1.0 — first public release

**MCP-native, self-hosted memory for agents, with enterprise governance in the write path.**

This is the first tagged release. It is a working end-to-end product, not a scaffold:
store, retrieve, MCP tools, redaction, audit, Docker, and an offline demo.

## Highlights

### Governance (the reason this exists)

- **Redaction before write.** 8 rule families (private keys, PRC resident ID, bank cards,
  JWT, API keys, credential assignments, e-mail, mainland phone numbers) run in
  `memory.Service.Add` **before** dedup, FTS and embedding. Plaintext never reaches the
  database in `enforce` mode, and it is not recoverable.
- **Append-only audit.** Every add / search / delete / reembed records actor, tenant,
  scope, memory id, timestamp, result count, `degraded`, redaction count. A database trigger
  rejects `UPDATE audit_log`. Search text is stored as **SHA-256 + length** by default.
- **One-command export.** `memalyze audit export --from … --to … --format csv|json`.
- **30-second demo, offline.** `memalyze demo` needs no Ollama, no network, no MCP client.

### Retrieval

- Vector-first with keyword backfill; `degraded` + `degrade_reason` instead of hanging.
- Measured on a 60-memory Chinese corpus: **recall@3 0.94 / MRR 0.92** with
  `qwen3-embedding:0.6b`; CJK bigram FTS5 at 0.88 recall. RRF was tested and rejected
  (worse than vector-only).
- Cold start fixed: warmup runs in the background (37.8 s), first real query 1.1 s.

### Deployment

- One static binary (~14 MB, CGO-free) for linux/darwin amd64+arm64 and windows amd64.
- MCP over **stdio** or **Streamable HTTP**; `/healthz` for probes.
- `docker compose up` runs memalyze + Ollama + a one-shot model pull.
- HTTP refuses to bind a non-loopback address without `MEMALYZE_HTTP_TOKEN` (fail closed).
- Four tools total: `memory_add`, `memory_search`, `memory_delete`, `echo`.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh
ollama pull qwen3-embedding:0.6b
claude mcp add --scope project memalyze -- memalyze
```

Or Docker:

```bash
printf 'MEMALYZE_HTTP_TOKEN=%s\n' "$(openssl rand -hex 24)" > .env
docker compose up -d --build
```

## Upgrade notes

First release — no migration path needed. Schema is at `user_version = 2`; opening an older
database from a pre-release build applies the audit-table migration automatically.

## Known limits (do not discover these in production)

- **No per-user authentication.** One bearer token per process; `actor` comes from
  `MEMALYZE_ACTOR`. One token = one trust boundary.
- **No TLS termination.** Put it behind a reverse proxy you control.
- **No encryption at rest.** The SQLite file is as protected as its filesystem.
- **No audit retention/purge** yet; export and archive yourself.
- **Redaction is pattern matching**, not a compliance certificate. It will not catch a name
  or a photographed ID card.
- **Brute-force vector scan** — fine up to roughly 10k memories.
- **Docker image not built in CI** for this tag (no daemon in the build environment); the
  Dockerfile and compose file are reviewed, and the same binary was verified over HTTP.

## Verify it yourself

```bash
memalyze demo                       # redaction + audit, 30 s, offline
memalyze audit export --format csv  # the CSV an auditor would ask for
go test ./...                       # if you build from source
```

Full threat model: `SECURITY.md`. Governance design: `docs/s15-redaction-audit.md`.

## Claude Desktop / MCPB bundles

Prebuilt `.mcpb` bundles (one per platform) are attached to this release. Download the one
matching your machine, open it with Claude Desktop, and it will ask where to keep the SQLite
file.

| Platform | Asset |
|---|---|
| macOS Apple Silicon | `memalyze-0.1.0-darwin-arm64.mcpb` |
| macOS Intel | `memalyze-0.1.0-darwin-amd64.mcpb` |
| Windows x64 | `memalyze-0.1.0-windows-amd64.mcpb` |
| Linux x64 | `memalyze-0.1.0-linux-amd64.mcpb` |

SHA-256 for all four: `MCPB-SHA256SUMS`.

These bundles have **not** been opened in Claude Desktop by the author (no macOS/Windows
machine available). The binary inside each is the same `v0.1.0` build as the standalone
assets. If the install sheet does not appear, use the standalone binary and please open an
issue with the platform and Claude Desktop version.
