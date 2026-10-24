# memalyze

**MCP-native, self-hosted memory for AI agents — with the governance an enterprise will actually ask for.**

One static binary. Local embeddings. No cloud, no graph database, no Python stack.
Redaction happens **before** anything is written; every read is **audited**.

```bash
./memalyze          # MCP over stdio, for Claude Code / Cline / any harness
```

[**v0.1.0 is out**](https://github.com/mahaoboy/memalyze/releases/tag/v0.1.0) — static
binaries for linux/darwin/windows, checksums published alongside.

![memalyze governance demo](docs/assets/demo.png)

---

## The problem

Every agent framework ships its own memory: a vector table here, a JSON file there, a
"notes" folder somewhere else. Three things follow, and all three hurt in production:

1. **You cannot audit it.** "Which customer's data did the agent read last Tuesday?" has no
   answer, so security review stops the project.
2. **You cannot deploy it offline.** Most options either call a cloud API or download model
   weights from HuggingFace at runtime — both fail inside a bank, a hospital, or a factory.
3. **You cannot keep it small.** Twenty-three tools, a graph database, and a Python
   dependency tree later, the "memory feature" is a second product to operate.

memalyze is the middleware that fixes those three, and nothing else.

## What is different

| | Cloud memory APIs | Framework built-ins | Personal note apps | **memalyze** |
|---|---|---|---|---|
| Where data lives | vendor cloud | inside the framework | local Markdown | **your server / your laptop** |
| How to connect | proprietary API | tied to one framework | MCP | **MCP, any harness, stdio or HTTP** |
| Redaction before write | ❌ | ❌ | ❌ | **✅ 8 rule families, configurable, irreversible by default** |
| Read audit | partial | ❌ | ❌ | **✅ append-only, per-actor, exportable** |
| Offline | ❌ | partial | ⚠️ model download needed | **✅ local embeddings, no network** |
| Deployment | SaaS | with the framework | 173 Python packages | **✅ one static binary / one compose file** |
| Tool surface | — | — | 23 tools | **4 tools** |

Not another mem0, and not Basic Memory either: those are good at *remembering*. memalyze
assumes remembering is becoming a commodity and competes on **governance, offline
operation, and a deliberately small surface**.

## 30-second demo

No Ollama, no network, no MCP client:

```bash
./examples/demo/demo.sh
# or: go run ./cmd/memalyze demo
```

It writes a memory containing a phone number, an e-mail and a resident ID, then shows:

- what is actually stored (`[REDACTED:phone]`, not `13800138000`);
- that searching for the original number returns **0 hits**;
- that a business-word search still hits, returning the redacted text;
- the audit rows for the two writes and the two reads.

Details and transcripts: [`docs/s15-redaction-audit.md`](docs/s15-redaction-audit.md).

## Quick start

### 1. Binary + local embedding model

```bash
make build
ollama pull qwen3-embedding:0.6b      # ~640 MB, 1024-dim, strong on Chinese
./memalyze --version
```

### 2. Wire it into a harness

Claude Code (project scope):

```bash
claude mcp add --scope project memalyze -- "$(pwd)/memalyze"
cp examples/claude-code/CLAUDE.md ./CLAUDE.md   # makes Claude search before answering
```

Cline: copy `examples/cline/cline_mcp_settings.json` and `.clinerules`.
Generic harness: [`examples/custom-harness/`](examples/custom-harness) — one prompt contract
plus pseudocode. All options: [`examples/README.md`](examples/README.md).

### 3. Or run it in Docker (memalyze + Ollama + model pull)

```bash
printf 'MEMALYZE_HTTP_TOKEN=%s\n' "$(openssl rand -hex 24)" > .env
docker compose up -d --build
curl -s http://127.0.0.1:8080/healthz     # {"status":"ok","version":"dev"}
```

Point a Streamable-HTTP MCP client at `http://127.0.0.1:8080/mcp` with
`Authorization: Bearer $MEMALYZE_HTTP_TOKEN`. See [`docs/s16-docker.md`](docs/s16-docker.md).

## Tools

Four, on purpose. Every extra tool is a decision the model has to make.

| Tool | What it does | Notable semantics |
|---|---|---|
| `memory_add` | store a memory | duplicates return the existing id; embedding failure keeps the memory (`embedding_failed=true`); returns `redacted` + rule names |
| `memory_search` | retrieve | vector first, keyword backfill, `degraded` + `degrade_reason` when the vector path is unavailable; empty query lists recent |
| `memory_delete` | soft delete | idempotent for already-deleted ids; unknown id returns a tool error |
| `echo` | wiring check | proves the MCP link works; no business logic |

There is **no** `memory_context` tool: `memory_search` already returns text you can paste
into a prompt, and two overlapping tools make model choice worse (D-017).

## Governance

**Redaction (write path).** Rules run in `memory.Service.Add` *before* dedup, FTS and
embedding — so plaintext never reaches the main table, the full-text index or a vector.

| Rule | Example input | Stored as |
|---|---|---|
| `private_key_block` | `-----BEGIN RSA PRIVATE KEY-----…` | `[REDACTED:private_key]` |
| `id_card_cn` | `110101199003071234` | `[REDACTED:id_card]` |
| `bank_card` | `6222021234567890123` | `[REDACTED:bank_card]` |
| `jwt` | `eyJhbGciOi…` | `[REDACTED:jwt]` |
| `api_key_known_prefix` | `sk-…`, `ghp_…`, `AKIA…` | `[REDACTED:api_key]` |
| `credential_assignment` | `api_key=abcdef123456` | `api_key=[REDACTED:credential]` |
| `email` | `a@b.com` | `[REDACTED:email]` |
| `phone_cn` | `+86 13800138000` | `[REDACTED:phone]` |

Modes: `enforce` (default), `dry-run` (count only, for tuning), `off` (debugging).
Custom rule packs are JSON: [`examples/redaction-rules.json`](examples/redaction-rules.json).

**Audit (read path).** Append-only `audit_log` (a database trigger rejects `UPDATE`):
actor, tenant, scope, action, memory id, timestamp, result count, `degraded`, redaction
count. The search text itself is stored as **SHA-256 + length** unless you explicitly set
`MEMALYZE_AUDIT_QUERY_PLAIN=1`.

```bash
memalyze audit export --from 2026-10-01 --to 2026-10-31 --format csv --out audit.csv
memalyze audit export --tenant acme --format json
```

**What is not here yet:** per-user authentication (one token per process, `actor` from
env), audit retention/purge, reversible redaction, audit UI.

## Retrieval, measured

Numbers from S8 on a 60-memory Chinese corpus, 16 queries, measured not assumed
([`docs/s8-retrieval-report.md`](docs/s8-retrieval-report.md)):

| Approach | recall@3 | MRR |
|---|---|---|
| `qwen3-embedding:0.6b` (default) | **0.94** | **0.92** |
| CJK bigram FTS5 (keyword backfill) | 0.88 | — |
| `nomic-embed-text` | 0.47 | — |

Two counter-intuitive findings that shaped the design: SQLite FTS5's default tokenizer
cannot search Chinese at all (0.19 recall until we stored bigram text), and RRF fusion
scored *worse* than vector-only, so v0.1 does not fuse. See D-012.

Latency (S14, cold start fixed): warmup 37.8 s in background, then search **1.14 s** with
`degraded=false`; after five idle minutes a fresh process answers in 1.52 s. If the vector
path is unavailable, search degrades to keyword in ≤3 s instead of hanging (D-015/D-018).

## How it works

```
harness (Claude Code / Cline / your agent)
   │  MCP: stdio or Streamable HTTP
   ▼
internal/mcp     4 tools, argument conversion only
   ▼
internal/memory  redact → dedup → store → embed; search: vector → keyword → degraded
   ├── internal/govern     rule engine (8 families, JSON configurable)
   ├── internal/store      interface; sqlite impl (memories, embeddings, audit_log)
   ├── internal/embed      float32 BLOB codec, cosine; Ollama client
   └── internal/search     CJK bigram tokenizer + FTS5 MATCH builder
```

- SQLite (WAL) with ULID ids, partial unique index for dedup, soft delete, TTL.
- Vectors are `float32` little-endian BLOBs; search is brute-force cosine (fine under ~10k
  memories, and the honest limit of a single-binary design).
- Schema migrations are versioned via `PRAGMA user_version` (v1 → v2 adds the audit table).

## Operations

```bash
memalyze audit export --from 2026-10-01 --format csv --out audit.csv   # who read what
memalyze backup --out /var/backups/memalyze-$(date +%F).db             # consistent snapshot
memalyze health --url http://127.0.0.1:8080/healthz                    # probe
```

- **systemd**: [`deploy/systemd/memalyze.service`](deploy/systemd/memalyze.service) +
  [`docs/s16-systemd.md`](docs/s16-systemd.md) (hardened unit, env file, backup timer).
- **Docker**: [`docs/s16-docker.md`](docs/s16-docker.md).
- `backup` uses `VACUUM INTO`, so snapshots are consistent under WAL and include the audit
  table; it refuses to overwrite an existing file.

## Configuration

All through environment variables; defaults work out of the box.

| Variable | Default | Meaning |
|---|---|---|
| `MEMALYZE_DB` | `./memalyze.db` | SQLite path |
| `MEMALYZE_TENANT` | `default` | default tenant for tool calls |
| `MEMALYZE_OLLAMA_URL` | `http://127.0.0.1:11434` | local embedding endpoint |
| `MEMALYZE_EMBED_MODEL` | `qwen3-embedding:0.6b` | embedding model |
| `MEMALYZE_QUERY_TIMEOUT` | `3s` | vector-path budget before degrading |
| `MEMALYZE_KEEP_ALIVE` | `30m` | Ollama model residency |
| `MEMALYZE_REDACT_MODE` | `enforce` | `enforce` / `dry-run` / `off` |
| `MEMALYZE_REDACT_RULES` | *(built-in)* | path to a JSON rule pack |
| `MEMALYZE_AUDIT` | `on` | append audit rows |
| `MEMALYZE_ACTOR` | `local` | actor recorded in audit |
| `MEMALYZE_AUDIT_QUERY_PLAIN` | `off` | store query text (default: hash only) |
| `MEMALYZE_HTTP_ADDR` | *(empty = stdio)* | e.g. `:8080` for Streamable HTTP |
| `MEMALYZE_HTTP_TOKEN` | *(empty)* | Bearer token; **required** when binding off-loopback |

## Status

Honest snapshot — this is an early-stage project, not a 1.0.

| Milestone | State |
|---|---|
| M0 立项与定位 (S1–S4) | ✅ |
| M1 备料与技术验证 (S5–S8) | ✅ |
| M2 核心链路 (S9–S14) | ✅ |
| M3 打磨与文档 (S15–S19) | ✅ S15 governance, S16 Docker, S17 docs, S18 tests+CI, S19 examples |
| M4 发布 (S20–S23) | 🟡 S20 ✅, S21 ✅ (**v0.1.0 public**), S22 GitHub ✅ / HN·Reddit·V2EX pending, S23 ⬜ |
| M5 复盘 (S24) | ⬜ needs real users |
| M5 复盘 (S24) | ⬜ |

Known gaps: no per-user auth, no audit retention, the Docker image has not been built in
CI (no daemon in the dev environment), brute-force vector scan, Chinese-first defaults
(English works, but the tokenizer is tuned for CJK).

## Docs

| Doc | What it covers |
|---|---|
| [`docs/s15-redaction-audit.md`](docs/s15-redaction-audit.md) | rules, modes, audit schema, demo transcript |
| [`docs/s16-docker.md`](docs/s16-docker.md) | compose stack, token policy, verification status |
| [`docs/s16-systemd.md`](docs/s16-systemd.md) | hardened systemd unit, env file, backup/restore |
| [`docs/s8-retrieval-report.md`](docs/s8-retrieval-report.md) | the 11 retrieval approaches and why this one won |
| [`docs/s13-context-injection.md`](docs/s13-context-injection.md) | the prompt contract that makes models search |
| [`docs/s14-end-to-end.md`](docs/s14-end-to-end.md) | real harness, two sessions, cold-start numbers |
| [`docs/s21-distribution.md`](docs/s21-distribution.md) | release artifacts, channels, post-release checklist |
| [`docs/s21-submissions.md`](docs/s21-submissions.md) | copy-paste entries for MCP directories and ClawHub |
| [`docs/s23-feedback-log.md`](docs/s23-feedback-log.md) | live feedback log (currently empty, honestly) |
| [`docs/s24-review.md`](docs/s24-review.md) | the milestone review template (not yet filled) |
| [`SECURITY.md`](SECURITY.md) | threat model: what is defended, what is out of scope |

## Non-goals

- Not a SaaS: you self-host, we do not hold your data.
- Not an agent framework: it never takes over the harness.
- Not a knowledge graph: no entity extraction pipeline, no graph database.
- Not feature-maximal: four tools, one binary, and a hard "no" to anything that needs an
  operator.

## License

AGPL-3.0-or-later (`LICENSE`). Commercial licensing for embedding in closed products:
open an issue titled `commercial license`.

Contributions: see [`CONTRIBUTING.md`](CONTRIBUTING.md). Security reports: see
[`SECURITY.md`](SECURITY.md).
