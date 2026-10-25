# S24 — Milestone review (template)

- **Status**: not started — requires real users (S22 posted, S23 filled)
- **Milestone**: M5 / S24
- **Rule**: the review is written from **artifacts**, not from feelings: numbers from this
  file, quotes from `docs/s23-feedback-log.md`, commits from `git log`.

## 1. Scorecard (fill in, do not estimate)

| Target (from 目标与指标) | Target | Actual | Verdict |
|---|---|---|---|
| 保底 | ≥30 stars, ≥3 self-deployed instances | | |
| 目标 | ≥100 stars, ≥10 instances, ≥20 feedback items | | |
| 理想 | ≥1 paying customer or $1k MRR | | |
| North star | weekly active deployments | | |
| Guardrail | ≤2 h/day, ≤¥10k/year, 0 compliance incidents | | |

Supporting numbers: `go test ./...` coverage, binary size, cold-start seconds, search p50,
open issues, mean time to first response.

## 2. The five questions

1. **Did anyone deploy it without talking to me?** (If no: the README/installer is the
   product, not the code.)
2. **Which sentence made them try it?** (Positioning evidence, not vanity.)
3. **What did they ask for that we deliberately refuse?** (If ≥3 people ask for the same
   refused thing, the refusal is the problem.)
4. **Where did redaction produce false positives or false negatives?** (Governance
   credibility is the whole wedge.)
5. **What did I spend the most hours on, and did it move the north star?**

## 3. Decision options

| Option | When it is right | Cost |
|---|---|---|
| **继续** (default) | ≥3 deployments, install works, feedback is about depth not direction | keep executing M4/M5 leftovers |
| **调整** | people deploy but for a different job than expected (e.g. team knowledge base, not agent memory) | rewrite positioning + 1–2 features; 2–4 weeks |
| **收窄** | governance is valued, retrieval is not (or vice versa) | cut a pillar; smaller surface |
| **转向** | no deployment after 4 weeks of active distribution (R1) | keep the engine (store/embed/MCP), change the wedge |

Write the chosen option as a `D-XXX` entry with 背景 / 选项 / 决定 / 理由 / 影响, and any new
discovery as an `O-XXX`. Never delete the previous decision; mark it 已修订.

## 4. Stop-loss check (from the task board)

- [ ] week 6: core loop done? (it was — S14 on 2026-10-20)
- [ ] week 12: ≥50 stars and ≥1 real user?
- [ ] any month with zero commits? (R4 trigger)

If a stop-loss line is hit, the review's job is to **act**, not to explain.

## 5. Output (one page, paste into the project journal)

```
S24 review — <date>
North star: N deployments (target N)
Scorecard: 保底 pass/fail | 目标 pass/fail | 理想 pass/fail
Biggest surprise: <one line>
Biggest waste: <one line>
Decision: 继续 / 调整 / 收窄 / 转向  → D-XXX
Next 12 weeks: <three bullets>
```
