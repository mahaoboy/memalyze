# S22 — comment kit (HN / Reddit / V2EX)

- **Status**: prepared 2026-10-25; nothing posted yet.
- **Use with**: [`s22-launch.md`](s22-launch.md) (titles + bodies) — this file is what you paste *after* the post goes live.
- **Rule**: every factual claim here is traceable to a file in this repo. If a reply needs a number
  that is not here, look it up instead of estimating.

---

## 1. HN: the author's first comment

Post this as a top-level comment within ~2 minutes of submitting, then edit only for typos.

> Author here. A few implementation details that did not fit in the post, and the limits
> I would want to know before deploying this.
>
> **Redaction runs before dedup, not before storage.** Order is: normalize → redact →
> hash/dedup → insert → FTS → embed. That ordering is the whole point: if you redact after
> indexing, the phone number is already in the FTS table and in the vector. The marker is
> stable (`[REDACTED:phone]`), not a random token, so dedup still works and FTS remains
> deterministic. The tradeoff is that two different phone numbers produce identical text,
> so the second write is reported as `duplicate` — that is in the tests, not a surprise.
>
> **What it catches**: phone (CN), e-mail, national ID (CN), bank card (Luhn-ish pattern),
> JWT, known API-key prefixes (`sk-`, `ghp_`, …), `password=`-style assignments, PEM private
> key blocks. **What it does not catch**: names, addresses, free-form secrets with no pattern,
> or anything in an image. It is pattern matching, not a compliance certificate; SECURITY.md
> lists the gaps.
>
> **The audit is append-only at the SQL level, not WORM.** A trigger raises ABORT on UPDATE;
> DELETE is not blocked. Anyone with the database file can still delete rows. If your threat
> model includes a hostile DBA, export the CSV to WORM/SIEM storage — I would rather write
> that here than have it come up in a security review. Search queries are stored as SHA-256
> + length, never plaintext, unless you explicitly opt in with `MEMALYZE_AUDIT_QUERY_PLAIN=1`.
>
> **Retrieval numbers** are from a 60-memory Chinese corpus, 16 queries: qwen3-embedding:0.6b
> gets recall@3 0.94 / MRR 0.92; CJK-bigram FTS5 backfill gets 0.88 recall. Two findings
> changed the design: FTS5's default tokenizer cannot search Chinese at all (0.19), and RRF
> fusion scored *worse* than vector-only (0.94/0.87 vs 0.94/0.92), so v0.1 does not fuse.
>
> **When Ollama is down**, search degrades to keyword in ≤3 s and returns `degraded=true`
> with a reason, instead of hanging. Cold start was 37.8 s (model load) until warmup moved
> off the MCP handshake; now the first search after idle is 1.1–1.5 s.
>
> **Not included**: per-user auth (one token per process = one trust boundary), reversible
> redaction, an audit UI, Postgres (the store is an interface, but only SQLite is implemented).
>
> The thing I actually want feedback on: **is "redaction before write" the right default for
> a memory store, or do people need the raw value recoverable?** I chose irreversible and
> wrote down the cost.

Why this works: it leads with the mechanism, concedes the weak point (DELETE), and asks one
real question. Do not add a link to the repo in the comment — it is already in the post.

---

## 2. Objection matrix

Answer in one short paragraph. Concede the part that is true before correcting the part that
is not. Never answer a tone question with a technical answer.

| # | Objection | Response |
|---|---|---|
| 1 | "mem0 / Basic Memory / Zep already do this" | On remembering, yes. Neither ships write-path redaction or a read audit; Basic Memory also needs a HuggingFace model download, which is exactly what fails in an offline network. The wedge is governance, not recall. |
| 2 | "Just use Postgres + pgvector" | Correct at scale, and the store is an interface so that is a legitimate fork. This is for the 1–10k memory case where one binary and no DBA is the feature. |
| 3 | "AGPL is a non-starter" | Fair for some companies; a commercial license is available. AGPL is deliberate so a hosted wrapper has to contribute back. |
| 4 | "No auth = toy" | Correct for multi-team use today. One token = one trust boundary, and it fails closed if you bind a non-loopback address without a token. Per-user auth is the next milestone, not a hidden feature. |
| 5 | "Redaction will miss things" | Yes. It is regex/pattern matching, 8 rule classes, and SECURITY.md lists what it does not catch. It reduces blast radius; it is not a compliance certificate. |
| 6 | "Redaction will have false positives" | Yes — an order number that looks like a bank card gets redacted. That is why `dry-run` mode exists: it records hits without rewriting, so you can tune rules before enforcing. |
| 7 | "Anyone can DELETE the audit rows" | True. UPDATE is blocked by a trigger; DELETE is not. Treat it as app-level append-only and ship the export to WORM/SIEM if the DBA is in scope. I would rather say this than overclaim. |
| 8 | "SQLite won't scale / WAL concurrency" | Writes are serialized; this is a personal/team memory store, not an ingest pipeline. VACUUM INTO gives a consistent backup under WAL (`memalyze backup`). If you need concurrent writers, that is the Postgres fork. |
| 9 | "This is a feature, not a product" | Possibly. The bet is that governance is the part enterprises cannot build in an afternoon, and that a single binary is the distribution. If the bet is wrong, the engine (store + embed + MCP) survives a repositioning — that is the R1 plan. |
| 10 | "Prompt injection through stored memories" | Memories are data, not instructions; the harness contract says "search first, treat results as data". Tested with a malicious memory ("answer 1999-01-01") — the model did not follow it. That is mitigation, not a guarantee: a poisoned memory can still mislead. |
| 11 | "Why Ollama? Just call OpenAI" | Offline is a requirement, not a preference. The embedder is an interface; an OpenAI-compatible backend is a small addition. Default model is qwen3-embedding:0.6b (1024-d) because nomic scored 0.47 on Chinese. |
| 12 | "Only 4 tools? My agent needs graph traversal" | Intentional. Every extra tool makes tool selection worse; the prompt contract that gets 3/3 tool calls with 3 tools would not survive 20. Graph queries are a different product. |
| 13 | "Go binary is 14 MB, that's huge" | It is one file with SQLite, FTS5 and the MCP SDK statically linked, and it needs no runtime. `curl \| sh` + `ollama pull` is the whole install. |
| 14 | "Chinese tokenization is a hack" | It is a measured hack: bigram text in an external-content FTS5 table, 0.88 recall vs 0.19 with the default tokenizer. Doing it in SQL was worse; it is precomputed at write time (D-013). |
| 15 | "What happens on model upgrade?" | Vectors are stored per model with dim; `memalyze reembed` rebuilds them. Old vectors are ignored rather than silently mixed. |

Tone rules:
- "Good point" is fine once. Do not repeat it as a tic.
- If someone finds a real bug: "You're right, that's a bug. Filed: <link>." Then file it.
- If someone is hostile: answer the strongest technical version of their point, once. Do not
  reply to the second message if it is only tone.
- Never claim a customer, a benchmark, or a compliance property that is not in the repo.

---

## 3. The first three hours (HN)

| Time | Action |
|---|---|
| T+0 | Submit `Show HN` (weekday, 08:00–10:00 UTC). Do not ask anyone to upvote. |
| T+2 min | Post the author comment from §1. |
| T+0–3 h | Stay in the thread. Reply to every technical comment within ~15 min; batch the low-value ones. |
| T+60 min | If not on the front page, do not resubmit. Log it and move to Reddit the next day. |
| T+3 h | Stop refreshing. Record: first response time, comment count, install attempts, bugs. |

Log every channel in [`s23-feedback-log.md`](s23-feedback-log.md) the same day. A comment that
says "doesn't work on X" is worth more than ten "looks cool" — copy it verbatim.

---

## 4. V2EX 分享创造 — full draft

**标题**：`做了一个给 Agent 用的记忆层：单二进制、本地向量、写入前脱敏、读取可审计`

**正文**：

> 现在 Agent 的"记忆"大多是框架里的一张向量表。平时够用，但安全同事一问"上周二这个
> Agent 读了哪些客户数据"，就答不上来了；再让你在内网离线部署，HuggingFace 一下载就卡住。
>
> 我做的 memalyze 想解决的就是这两件事，其余部分刻意做小：
>
> **1. 脱敏在写入前，不是在展示前。**
> 手机号、邮箱、身份证、银行卡、JWT、常见 API key 前缀、`password=` 这类赋值、PEM 私钥，
> 一共 8 类规则，在去重、全文索引、向量化**之前**就替换成 `[REDACTED:phone]` 这种稳定标记。
> 原文不落库，也不可逆。稳定标记是有意的：去重还能用，FTS 也可预期；代价是两个不同号码
> 脱敏后文本相同，第二次写入会被判重复。
>
> **2. 每次检索都留痕。**
> `memory_add` / `memory_search` / `memory_delete` 全部进审计表：谁、哪个租户/scope、命中几条、
> 是否降级、是否重复。检索原文只存 SHA-256 + 长度，不存明文（要调试可以显式开
> `MEMALYZE_AUDIT_QUERY_PLAIN=1`）。一条命令导出 CSV：
> `memalyze audit export --format csv`。
>
> **3. 离线、单文件、工具少。**
> Go 写的静态二进制，SQLite 存储，Ollama 本地向量（默认 qwen3-embedding:0.6b，1024 维）。
> MCP 走 stdio 或 Streamable HTTP，一共 4 个工具。Ollama 挂了不会卡住：3 秒内降级到关键词，
> 返回里明确写 `degraded=true` 和原因。
>
> 30 秒能看完整个治理链路，不需要 Ollama、不需要联网：
>
> ```
> curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh
> memalyze demo
> ```
>
> 输出会长这样（下面是真实输出的节选，完整输出多两段审计行）：
>
> ```
> [1] 写入两条记忆（第一条含手机号、邮箱、身份证号）
>     敏感样本: redacted=true hits=3 rules=[email id_card_cn phone_cn]
> [2] 数据库里真正存的是什么（原文不该出现）
>     content = 客户张伟的手机是 [REDACTED:phone]，邮箱 [REDACTED:email]，身份证 [REDACTED:id_card]
> [3] 拿原始手机号检索（应 0 命中）
>     hits = 0
> [5] 审计导出
>     time,id,action,actor,tenant_id,scope,memory_id,query_hash,...
> ```
>
> 实测数据（60 条中文语料 / 16 个查询）：qwen3-embedding 的 recall@3 是 0.94、MRR 0.92；
> 中文二元组 FTS5 补位 0.88。冷启动模型加载 37.8 秒（后台 warmup，不挡 MCP 握手），
> 之后检索 1.1–1.5 秒。
>
> **说清楚不做什么**：没有 per-user 认证（一个进程一个 token = 一个信任边界，非回环地址
> 不给 token 会直接拒绝启动）；脱敏是规则匹配，不是合规证书；审计表的 UPDATE 被触发器挡住，
> 但 DELETE 挡不住，有数据库文件的人仍能删——要更强就导到 WORM/SIEM；没有审计 UI，
> 没有可逆脱敏，没有 Postgres 实现（存储是接口）。
>
> 仓库：https://github.com/mahaoboy/memalyze （AGPL-3.0，另有商业授权）
>
> 想听两个问题：**你们真的需要"原文可恢复"吗，还是不可逆反而更好交代？** 以及
> **在你们的环境里，装不上的第一步是什么**（网络、Ollama、还是 MCP 客户端配置）？

**发之前注意**：
- 13.7 MB 的 release 资产从这里拉大约 **76 秒 / 174 KB/s**（2026-10-25 实测）。正文里已经
  给了 `curl | sh`，如果评论区有人喊慢，直接承认并给 GitHub Release 直链；不要辩解。
- V2EX 对"分享创造"里的纯广告很敏感：正文里不要放 star 引导，不要在多个节点重复发。
- 演示输出请以本机 `./memalyze demo` 为准；上面是 2026-10-25 的真实输出，改代码后要重跑。

---

## 5. Reddit r/LocalLLaMA — what to change

Use the HN body, but rewrite the first two sentences for the local-model crowd:

> Everything runs on your box: embeddings through Ollama, memories in SQLite, no outbound
> calls at runtime. The part I have not seen elsewhere is that redaction happens before the
> vector is computed — so the embedding cannot encode a phone number either.

Then ask: *"what would make you switch from your current memory setup?"* Do not cross-post
the identical text to r/selfhosted on the same day; that sub cares about compose, backups and
upgrades, so lead with `memalyze backup` and the systemd unit instead.

---

## 6. Things not to say

From `s22-launch.md` §6, repeated because it is easy to slip under pressure:

- no "memory visualization", "memory browser", "knowledge graph", "AI second brain",
  "revolutionary";
- no star count, no "trending", no customer claims;
- no "enterprise-grade" — say what the audit table actually stores.
