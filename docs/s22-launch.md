# S22 — Launch package

- **Status**: copy ready (2026-10-25); **nothing posted yet** — posting needs your accounts
- **After posting**: use [`s22-comment-kit.md`](s22-comment-kit.md) (author's first comment, 15 objections, 3-hour runbook, full V2EX draft)
- **Milestone**: M4 / S22
- **Acceptance**: GitHub public + ≥3 channels. Only the first is fully under our control.

## 1. Positioning, in one sentence

**EN:** The memory layer for agents that a security team will actually approve: one static
binary, local embeddings, redaction before write, audit on every read.

**ZH:** 给 Agent 用的记忆层，安全团队会签字的那种：单二进制、本地向量、写入前脱敏、
每次读取都留痕。

Three proof points, in this order (each is verifiable in under a minute):

1. `memalyze demo` — 30 s, no Ollama, shows plaintext absent and audit present.
2. `recall@3 0.94` on a Chinese corpus, measured in `docs/s8-retrieval-report.md`.
3. Four tools, one binary, `curl | sh`, `docker compose up`.

## 2. Hacker News

**Title (pick one):**

- `Show HN: Memalyze – self-hosted agent memory with redaction and audit`
- `Show HN: A memory layer for agents that passes a security review`

**Body:**

> Most agent "memory" is a vector table bolted onto a framework. That is fine until someone
> from security asks which customer data the agent read last Tuesday, or until you try to
> deploy inside a network with no outbound access.
>
> memalyze is a single Go binary that speaks MCP (stdio or Streamable HTTP), stores memories
> in SQLite, and embeds locally with Ollama (qwen3-embedding, 1024-d). Two things are
> different:
>
> - **Redaction runs before anything is written.** Phone numbers, e-mail, resident ID, bank
>   cards, API keys and private keys are replaced with stable markers before dedup, FTS and
>   embedding. The plaintext is never stored, so it is not recoverable — by us or by you.
> - **Every read is audited.** Append-only table, per actor/tenant/scope, query stored as
>   SHA-256 + length. `memalyze audit export --format csv` hands it to an auditor.
>
> Four tools total (`memory_add`, `memory_search`, `memory_delete`, `echo`). No graph
> database, no Python, no SaaS. Retrieval is vector-first with keyword backfill and an
> explicit `degraded` flag instead of a 60-second hang.
>
> `memalyze demo` runs the whole governance story in 30 seconds without Ollama or network.
>
> What it is not: per-user auth (one token per process), reversible redaction, or an audit
> UI. Those are written down in SECURITY.md rather than implied away.
>
> Repo: https://github.com/mahaoboy/memalyze — feedback on the threat model especially
> welcome.

## 3. Reddit — r/LocalLLaMA

**Title:** `Self-hosted memory for agents, fully offline: redaction before write + audit on read`

**Body:** shorter, local-model-first, no marketing adjectives. Lead with the offline story
(no HuggingFace download at runtime, no cloud API), then the governance angle, then ask a
specific question: *"what would make you switch from your current memory setup?"*

Do not cross-post the same text to r/selfhosted on the same day; rewrite the opening for
each community (selfhosted cares about compose/backup, LocalLLaMA cares about models and
VRAM).

## 4. V2EX（中文，分享创造节点）

**标题：** `做了一个给 Agent 用的记忆层：单二进制、本地向量、写入前脱敏、读取可审计`

**开头三句**（中文社区最烦翻译腔，直接说问题）：

1. 现在 Agent 的"记忆"大多是框架里的一张向量表，安全同事一问"上周二读了哪些客户数据"就答不上来。
2. 所以我把治理做进了写入路径：手机号/邮箱/身份证/银行卡/API key 在**落库前**就替换成标记，原文不存，也不可逆。
3. 每次检索都写审计（谁、哪个租户、命中几条、是否降级），一条命令导出 CSV。

结尾放 `memalyze demo` 和 README，明确写"还没有 per-user 认证，一个进程一个 token"。

## 5. 即刻 / X thread

1. 一条命令看完全部卖点：`memalyze demo`（不需要 Ollama、不需要联网）
2. 为什么脱敏必须在写入前：FTS 和向量都会记住原文 → 截图 `docs/assets/demo.png`
3. 审计长什么样：一条 CSV，query 只有 hash → 贴 4 行样例
4. 单二进制 14MB，4 个工具，MCP stdio 或 HTTP
5. 已知缺口（无 per-user auth、无审计保留策略）+ 仓库链接

## 6. Language rules (learned from O-008)

Never say: "memory visualization", "memory browser", "knowledge graph", "AI second brain",
"revolutionary". PilotDeck and others already own the visualization framing; repeating it
invites a comparison we lose.

Always say: **governance, audit, redaction, offline, single binary, four tools**.

## 7. Response playbook

| Objection | Response |
|---|---|
| "mem0 / Basic Memory already do this" | Agreed on remembering. Neither ships write-path redaction or an append-only read audit; that is the whole product (and Basic Memory needs a HuggingFace download, which fails offline). |
| "Just use Postgres + pgvector" | Reasonable at scale. This is for the 1–10k memory case where you want one binary and no DBA; the store interface leaves room for a Postgres impl. |
| "AGPL is a non-starter" | Commercial license available; the AGPL choice is deliberate so the hosted-wrapper case stays fair. |
| "No auth is a dealbreaker" | Correct for multi-team use today. One token = one trust boundary; per-user auth is the next milestone, not a hidden feature. |
| "Redaction will miss things" | Yes — it is pattern matching, not a compliance certificate. SECURITY.md lists exactly what it does not catch. |

## 8. Success metrics and the stop-loss line

| Window | Signal | Reading |
|---|---|---|
| 72 h | install issues, demo failures | fix before any more posting |
| 2 weeks | ≥10 real self-deployments, ≥20 pieces of feedback | on track for M5 |
| 4 weeks | **0 real users** | triggers **R1**: rethink positioning, not marketing (see risk register) |
| 12 weeks | <50 stars and no users | S24 stop-loss: change the wedge, keep the engine |

Track in `docs/s23-feedback.md`, not in memory.

## 9. Posting checklist

- [ ] tag `v0.1.0`, wait for the release workflow, verify assets + install
- [ ] README's three steps verified on a clean machine
- [ ] `memalyze demo` output matches `docs/assets/demo.png`
- [ ] SECURITY.md and CONTRIBUTING.md present (they are what senior readers check first)
- [ ] HN: post 08:00–10:00 UTC on a weekday, then stay in the thread for 3 hours
- [ ] Reddit: next day, rewritten opening
- [ ] V2EX: same week, Chinese, no cross-links spam
- [ ] log every channel + timestamp + first response time in the feedback file
