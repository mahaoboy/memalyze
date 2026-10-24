# 30-second governance demo

Two questions every enterprise buyer asks before storing customer data in an agent:

1. **"Will our customer data leak into your database?"**
2. **"When something happens, can you tell me who read what?"**

This demo answers both in one command, without Ollama, network, or an MCP client.

```bash
./examples/demo/demo.sh
```

What it does (each step maps to an S15 acceptance criterion):

| Step | Shows |
|---|---|
| 1 | `memory_add` with a phone number, e-mail and resident ID inside the text |
| 2 | the **stored** text — markers instead of plaintext |
| 3 | searching for the original phone number returns **0 hits** |
| 4 | searching by a business word hits, and returns the redacted text |
| 5 | `audit export` as CSV: actor, action, tenant, result count, `degraded` |

Expected output is reproduced in
[`docs/s15-redaction-audit.md`](../../docs/s15-redaction-audit.md#8-demo-transcript-memalyze-demo).

## Try your own rules

```bash
# preview only, nothing written
memalyze redact --text "EMP-004217 的手机是 13800138000"

# with a custom rule pack
MEMALYZE_REDACT_RULES=examples/redaction-rules.json memalyze redact --file sample.txt

# tune against your own traffic: count, do not modify
MEMALYZE_REDACT_MODE=dry-run memalyze demo
```

## What the demo deliberately does not claim

- No reversible redaction: once written, the original is gone (`docs/s15-redaction-audit.md` §6).
- No audit UI: `memalyze audit export --format json` is the interface.
- No per-user authentication yet: `actor` comes from `MEMALYZE_ACTOR`, so run one process
  per trust boundary until v0.2 introduces auth.
