# S23 — Feedback collection and triage

- **Status**: template ready (2026-10-25); no feedback yet (nothing published)
- **Milestone**: M4 / S23
- **Purpose**: turn raw comments into decisions instead of opinions. Anything that could
  change the plan becomes an `O-` entry in the project opportunity log **before** it becomes
  a decision.

## 1. Where feedback arrives

| Source | Who | Signal quality |
|---|---|---|
| GitHub issues | people who actually installed it | highest — they hit a real failure |
| HN / Reddit / V2EX comments | evaluators | high on positioning, low on bugs |
| Direct messages / e-mail | lurkers | high intent, no context |
| Install telemetry | nobody (telemetry is off by design) | — |

Because there is no telemetry, **ask for a self-report**: "if you deployed it, reply with
`memalyze --version` + one sentence about what you use it for". That is the north-star
metric (weekly active deployments) and it only exists if people answer.

## 2. One row per item (append-only)

| # | Date | Channel | Who (handle) | Verbatim (≤2 lines) | Category | Severity | Evidence | Action | Status |
|---|---|---|---|---|---|---|---|---|---|
| F-001 | 2026-10-25 | — | — | — | — | — | — | — | — |

Categories:

| Category | Meaning | Typical action |
|---|---|---|
| `install` | cannot get it running | fix docs/installer, highest priority |
| `bug` | works but wrong | reproduce → issue → fix |
| `governance` | redaction/audit gap or false positive | rule change or doc clarification; may become a decision |
| `ux` | works, painful | batch; do not fix one-off |
| `missing` | feature request | check the three constraints before even discussing |
| `positioning` | "I don't get why this exists" | signals for M5, not a code change |
| `praise` | — | quote in README only with permission |

Severity: `S1` data loss/leak, `S2` blocks install, `S3` degraded use, `S4` annoyance.

## 3. Triage rules

1. **S1/S2 same day**, everything else weekly (Saturday ritual, 2 h).
2. A feature request that would add a fifth tool needs a written justification before
   discussion — otherwise decline politely and link CONTRIBUTING.
3. Two independent reports of the same confusion = documentation bug, even if the docs are
   technically correct.
4. Any request that implies "host our data" is out of scope (D-004); say so once, kindly.
5. Never argue about AGPL in an issue thread; link the licensing paragraph.

## 4. Weekly rollup (5 lines, Saturday)

```
Week N: +X feedback (install a, bug b, governance c, ux d, missing e, positioning f)
New deployments: N   Stars: N   Issues open: N
Top blocker: <one line>
Decision needed: <one line> → O-XXX / D-XXX
```

## 5. Promotion path (the part that matters)

```
raw comment
   → F-row in this file
   → if it can change strategy: O-XXX in 00-总览/机会日志.md
   → if it changes a direction: D-XXX in 00-总览/决策日志.md
   → if it changes work: S-task in 00-总览/任务清单-总进度.md
```

A comment that never leaves this file is a comment that was never read twice.

## 6. First-week questions to ask explicitly

- Did the install work on the first try? If not, which step?
- What did you use it for — and what did you expect that it did not do?
- Did the redaction rules fire on anything legitimate (false positive)?
- Would you need per-user auth before sharing it with a teammate?

Answers to the last two decide whether S24 keeps the current wedge or narrows it further.
