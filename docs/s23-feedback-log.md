# Feedback log (live)

Method, categories and triage rules: [`s23-feedback.md`](s23-feedback.md).
This file is the raw record. Append only; never rewrite a row.

## Channel status

| Channel | Date | URL | Status | First response |
|---|---|---|---|---|
| GitHub Release `v0.1.0` | 2026-10-25 | https://github.com/mahaoboy/memalyze/releases/tag/v0.1.0 | ✅ published | — |
| HN Show HN | — | — | ⬜ not posted | — |
| Reddit r/LocalLLaMA | — | — | ⬜ not posted | — |
| V2EX 分享创造 | — | — | ⬜ not posted | — |
| awesome-mcp-servers PR | 2026-10-25 | [PR #17582](https://github.com/punkpeye/awesome-mcp-servers/pull/17582) | 🟡 awaiting review | — |
| MCP Registry (MCPB) | 2026-10-25 | [entry](https://registry.modelcontextprotocol.io/v0/servers?search=memalyze) | ✅ published | — |
| dsh / ClawHub skill | — | — | ⬜ not submitted | — |

## Baseline (2026-10-25, a few hours after publish, before any announcement)

| Metric | Value | Note |
|---|---|---|
| stars | 0 | no announcement yet |
| forks | 0 | |
| open issues | 0 | |
| release asset downloads | 2 per binary, 1 for `SHA256SUMS` | all from our own verification runs, not users |
| weekly active deployments (north star) | 0 (unknown) | no telemetry by design; needs self-report |

## Feedback queue

| # | Date | Channel | Who | Verbatim (≤2 lines) | Category | Severity | Evidence | Action | Status |
|---|---|---|---|---|---|---|---|---|---|
| — | — | — | — | *(empty until the first channel is posted)* | — | — | — | — | — |

## Weekly rollup

```
Week 0 (until 2026-11-01): release published, 0 channels announced
New deployments: 0   Stars: 0   Issues: 0
Top blocker: distribution, not product
Decision needed: post HN/Reddit/V2EX → S22 completion
```

## Notes to self

- Downloads from our own verification inflate the number; subtract them when reading.
- A star without an install is not feedback; ask "did you deploy it" before recording praise.
- The first install failure is worth more than ten "looks cool" comments — record it verbatim.

## 2026-11-17 — Outreach round 1 closed: 5 P1 threads, 0 replies

Read-only Gmail API check (account `mahaoboy@gmail.com`). Each thread was fetched with
`format=metadata`; every message's sender was verified individually, not inferred from the
unread list.

| Target | Thread ID | Messages | From us | From them | Last send |
|---|---|---|---|---|---|
| Dify | `1a183900325a99ad` | 3 | 3 | 0 | 2026-11-11 |
| Open WebUI | `1a1839054cf70940` | 3 | 3 | 0 | 2026-11-11 |
| Roo Code | `1a183906e61d2fbd` | 3 | 3 | 0 | 2026-11-11 |
| Jan | `1a183908cac3790f` | 3 | 3 | 0 | 2026-11-11 |
| Goose | `1a18390abe086c27` | 3 | 3 | 0 | 2026-11-11 |

Cross-checks:

- All 16 target domains, 120 days, `from:` filter → 0 inbound messages.
- `memalyze -from:mahaoboy@gmail.com` over 120 days → 0 results.
- Spam folder (30 days) → no outreach replies, only unrelated junk.
- No delivery failure for any outreach address; recent bounces are internal
  `test.dev` / `wenwen.dev` test mail only.
- Round totals: 27 sends across 11 contacted targets, 0 human replies.

Decision: close this round per the task-board rule. Do not send a 4th message to these
addresses. S22/S23 remain open only for non-email channels (Registry / awesome) and for the
first real deployment.

Next route: small integration shops and independent deployers, not platform `support@`
inboxes. First message offers a remote install plus a poisoned-memory demo, and asks for one
install record — not a meeting.
