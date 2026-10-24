# S15 — Enterprise governance: redaction before write, audit on read

- **Status**: done (2026-10-25)
- **Milestone**: M3 / S15
- **Predecessor**: S14 (`docs/s14-end-to-end.md`, commit `544957c`)
- **Scope**: write-path redaction, append-only audit, export command, 30-second demo.
  **No new MCP tools** (tool surface stays at 4), no audit UI, no reversible redaction.

## 1. Why this step exists

D-011 narrowed the differentiation to **enterprise governance + single-binary offline +
restrained tool surface**. S15 is the first half of that promise: an enterprise buyer's
first two questions are not "can you remember" but

1. "will my customer data leak into your store?" and
2. "when something happens, can you tell me who read what?"

Before S15 the repo had **zero** redaction and audit code — the README claimed
governance the binary did not provide. That gap is now closed.

## 2. Acceptance criteria and where they are met

| # | Criterion | Implementation | Test |
|---|---|---|---|
| 1 | Redaction **before write**; plaintext never lands in the main table, FTS or vectors | `memory.Service.Add` redacts before dedup/embed (`internal/memory/service.go`) | `TestAddRedactsBeforeStore`, `TestRedactionHappensBeforeDedup` |
| 2 | Irreversible by default; reversible requires a separate switch (not in v0.1) | No key material is stored anywhere in the write path | code review; `docs` §6 |
| 3 | Audit records add/search/delete with actor, tenant, scope, memory id, time, result count, `degraded`; **query stored as hash + length** | `internal/store/audit.go`, `internal/store/sqlite/audit.go` | `TestAuditRecordsAddSearchDelete`, `TestAuditQueryPlainWhenExplicitlyEnabled` |
| 4 | One command exports a time range as CSV/JSON; no new MCP tool | `memalyze audit export` | `internal/audit/export_test.go` |
| 5 | 30-second demo: sensitive data is absent, reads are recorded | `memalyze demo` | `examples/demo/demo.sh` |

## 3. Redaction rules

Built-in rule set (`internal/govern.DefaultRules`), applied **in this order** — long,
high-structure patterns first so a shorter rule cannot shred them:

| Rule | Catches | Replacement |
|---|---|---|
| `private_key_block` | PEM `BEGIN/END ... PRIVATE KEY` blocks | `[REDACTED:private_key]` |
| `id_card_cn` | PRC resident ID, 18 digits | `[REDACTED:id_card]` |
| `bank_card` | UnionPay / Visa / MasterCard 16–19 digits | `[REDACTED:bank_card]` |
| `jwt` | `eyJ…` three-part tokens | `[REDACTED:jwt]` |
| `api_key_known_prefix` | `sk-…`, `ghp_…`, `AKIA…`, `xox?-…` | `[REDACTED:api_key]` |
| `credential_assignment` | `api_key=…`, `password: …` — **keeps the key name** | `$1=[REDACTED:credential]` |
| `email` | RFC-ish e-mail addresses | `[REDACTED:email]` |
| `phone_cn` | mainland mobile, optional `+86` | `[REDACTED:phone]` |

Design notes:

- **No false positives on ordinary text** is an explicit test (`TestNoFalsePositiveOnNormalText`
  covers timestamps, ports, versions, prices, ULIDs, 12-digit order numbers, Chinese prose).
- Replacements are **stable markers**, so redaction is idempotent (a second pass hits
  nothing) and still readable to a human or a model.
- A custom rule file can add/override rules:

```json
{"rules": [
  {"name": "employee_id", "pattern": "\\bEMP-\\d{6}\\b", "replacement": "[REDACTED:employee_id]"},
  {"name": "phone_cn", "pattern": "...", "replacement": "[REDACTED:phone]", "enabled": false}
]}
```

`enabled: false` disables a single built-in rule without copying the whole set.

## 4. Modes

| Mode | Content stored | `redacted` in result | When to use |
|---|---|---|---|
| `enforce` (**default**) | redacted text | true + rule names | production, demos, customer trials |
| `dry-run` | **original** text | true + rule names + `dry_run=true` | tuning rules on your own traffic before trusting them |
| `off` | original text | false | debugging only |

Why `enforce` is the default: the S15 planning note said "run dry-run for a week first",
but that was a note about the author's own machine. Shipping `dry-run` as the product
default means **shipping a binary that writes phone numbers to disk by default** — exactly
the failure the feature exists to prevent. `dry-run` remains available per-deployment and
is recorded in the audit (`dry_run=1`), so a tuning week is still possible without
silently weakening the default. See D-020.

## 5. Audit log

Table `audit_log` (schema v2, append-only):

| Column | Meaning |
|---|---|
| `id`, `ts` | ULID + millisecond timestamp |
| `actor`, `tenant_id`, `scope` | who, for which tenant, in which scope |
| `action` | `add` / `search` / `delete` / `reembed` |
| `memory_id` | the row involved (null for searches) |
| `query_hash`, `query_len` | **SHA-256 + byte length**, not the query text |
| `result_count` | hits returned, or 1/0 for mutations |
| `degraded`, `duplicate`, `redacted`, `dry_run` | outcome flags |
| `detail` | `degrade=query_timeout`, `list=recent`, `error=…` — never memory text |

Guarantees:

- **Append-only at the database level**: `audit_no_update` trigger raises `ABORT` on any
  `UPDATE audit_log`. Application code only ever issues `INSERT`.
- **Best-effort, never blocking**: writes use `context.WithoutCancel` + a 2 s cap, so a
  timed-out search still produces its audit row; a failed audit only logs to stderr.
- **No plaintext by default**: `MEMALYZE_AUDIT_QUERY_PLAIN=1` is required to store the
  query itself (then it appears in `detail`).
- Failed operations are audited too (`result_count=0`, `detail=error=…`), because
  "someone tried to read memory X and failed" is also evidence.

## 6. What is deliberately not here

- Reversible redaction (would need a key + KMS story; v0.1 stores no key material).
- Audit UI, pagination, retention policy, WORM storage.
- New MCP tools — the surface remains `memory_add` / `memory_search` / `memory_delete` /
  `echo`.
- Data-residency features (R8 red line).

## 7. Configuration

| Variable | Default | Meaning |
|---|---|---|
| `MEMALYZE_REDACT_MODE` | `enforce` | `enforce` / `dry-run` / `off` |
| `MEMALYZE_REDACT_RULES` | *(empty)* | path to JSON rules; empty = built-in |
| `MEMALYZE_AUDIT` | `on` | `on` / `off` |
| `MEMALYZE_ACTOR` | `local` | actor written into every audit row |
| `MEMALYZE_AUDIT_QUERY_PLAIN` | `off` | store query text in `detail` (default: hash only) |

Bad rules **fail startup** instead of silently disabling redaction.

## 8. Demo transcript (`memalyze demo`)

```
[1] 写入两条记忆（第一条含手机号、邮箱、身份证号）
    敏感样本: id=01M5QF7EK6… redacted=true hits=3 rules=[email id_card_cn phone_cn] dry_run=false

[2] 数据库里真正存的是什么（原文不该出现）
    content = 客户张伟的手机是 [REDACTED:phone]，邮箱 [REDACTED:email]，身份证 [REDACTED:id_card]
    ✅ 手机号 / 邮箱 / 身份证号 均未落库

[3] 拿原始手机号检索（应 0 命中）
    hits = 0
    ✅ 库里没有任何可反查原始号码的索引

[4] 用业务词检索（命中，且返回脱敏文本）
    - 客户张伟的手机是 [REDACTED:phone]，邮箱 [REDACTED:email]，身份证 [REDACTED:id_card] (matched_by=[keyword])

[5] 审计导出（谁能读、读过什么，一条命令交差）
time,id,action,actor,tenant_id,scope,memory_id,query_hash,query_len,result_count,degraded,duplicate,redacted,dry_run,detail
2026-10-24T17:27:09Z,01M5QF7EK8…,add,demo,acme,ops,…,0,1,false,false,0,false,
2026-10-24T17:27:09Z,01M5QF7EK8…,add,demo,acme,crm,…,0,1,false,false,3,false,
2026-10-24T17:27:09Z,01M5QF7EK9…,search,demo,acme,,,ad4af312…,6,1,true,false,0,false,degrade=no_embedder
2026-10-24T17:27:09Z,01M5QF7EK9…,search,demo,acme,,,a6942f97…,11,0,true,false,0,false,degrade=no_embedder
```

One consequence worth knowing: two different phone numbers redact to the same marker, so
the second `add` is a **duplicate**. That is intended (dedup runs on stored text) and is
covered by `TestRedactionHappensBeforeDedup`. If you need per-customer uniqueness, put it
in `scope` or `metadata`, not in the redacted text.

## 9. Follow-ups

- [ ] audit retention/purge command (needed before the first enterprise contract)
- [ ] `reembed` audit assertion (code path exists; no test yet)
- [ ] rule pack for other jurisdictions (ID/passport/IBAN) — keep it optional, not default
- [ ] export to object storage / syslog for SIEM integration
