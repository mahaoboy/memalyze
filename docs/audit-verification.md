# Audit chain, checkpoints, and governance reports

v0.2.0 upgrades `audit_log` to schema v3. The goal is **tamper evidence with an explicit trust
boundary**, not a claim that a local SQLite file is immutable.

## 1. What schema v3 changes

Each event has three storage-generated fields:

| Field | Meaning |
|---|---|
| `seq` | contiguous sequence, starting at 1 |
| `prev_hash` | previous event's hash; the first event uses 64 zeroes |
| `hash` | SHA-256 over the versioned canonical event, including `prev_hash` |

`audit_no_update` and `audit_no_delete` reject both mutations at the SQL layer. A separate
`audit_no_replace` before-insert conflict guard prevents `INSERT OR REPLACE` from bypassing the
delete trigger when recursive triggers are disabled; the application also enables recursive
triggers as defense in depth. Application appends run inside `BEGIN IMMEDIATE`, so reading the
head and inserting the next event is one serialized operation. Callers cannot choose `seq`,
`prev_hash`, or `hash`.

Opening a schema-v2 database automatically migrates it. Legacy rows are ordered by `(ts, id)`,
assigned contiguous sequence numbers, and hashed into the new chain. The migration runs in one
transaction. Back up before upgrading; older binaries do not support schema v3.

**Migration baseline:** v0.2.0 cannot retroactively prove that pre-migration rows were complete
or authentic. It proves consistency of the migrated chain from that baseline forward.

## 2. Canonical hash format (`sha256-chain-v1`)

```text
hash = SHA256("memalyze-audit-event-v1\x00" || UTF8(canonical_json))
```

`canonical_json` is Go `encoding/json` output for a structure with fields in this exact order:

```text
version, seq, id, ts, actor, tenant_id, scope, action, memory_id, query_hash,
query_len, result_count, degraded, duplicate, redacted, dry_run, detail, prev_hash
```

- `version` is `1`; `ts` is Unix milliseconds.
- Optional strings are always present: an absent value is JSON `null`, not an omitted key.
  Empty optional values written by memalyze normalize to `null`. A legacy v2 `id=NULL` remains
  JSON `null`; a legacy empty-string ID remains `""`, so those distinct SQL values are preserved.
  New appends always generate a non-empty ULID.
- Booleans and integers are JSON primitives. Strings use Go's standard JSON escaping.
- `hash` itself is not part of the input. The genesis hash is 64 ASCII zeroes.

A fixed independent test vector is locked in `internal/store/sqlite/audit_chain_test.go`.
Changing this format requires a new algorithm/version and an explicit migration; it must not be
silently reinterpreted as `sha256-chain-v1`.

## 3. Verify the chain

```bash
# Recompute every event and check sequence/previous-hash/hash links.
memalyze audit verify --db ./memalyze.db
memalyze audit verify --db ./memalyze.db --json

# Save a new trust anchor. The target must not already exist; mode is 0600.
memalyze audit verify --db ./memalyze.db --write-checkpoint ./memalyze.audit.json

# Later, compare the complete database against that exact point in time.
memalyze audit verify --db ./memalyze.db --checkpoint ./memalyze.audit.json
```

A failed verification exits nonzero and reports stable reason codes such as
`sequence_mismatch`, `previous_hash_mismatch`, `hash_mismatch`, and `checkpoint_mismatch`.
Where applicable, `first_invalid_seq` identifies the first gap or invalid event.

### The checkpoint distinction is essential

| Result | What it proves |
|---|---|
| `ok=true`, `checkpoint_status=not_provided` | the remaining database chain is internally consistent |
| `ok=true`, `checkpoint_status=matched` | it also exactly matches the independently supplied checkpoint |
| `ok=false`, `checkpoint_status=mismatch` | event count, last sequence, head, algorithm, or schema differs |

Without a checkpoint, deleting a complete suffix leaves a valid shorter chain. A checkpoint
compares **exact** `event_count`, `last_seq`, and `head`, so it detects both tail deletion and
ordinary events appended after that checkpoint. Therefore an old checkpoint is expected to
mismatch a live database that has legitimately advanced; write and securely store a fresh one at
each backup/review point.

Checkpoints contain no memory content or audit detail, but they are **unkeyed and unsigned**.
Their integrity comes from where and how you store them. Keep them on a separate host, an
append-only/object-locked store, or another system the database writer cannot modify. If an
attacker can rewrite both the database and every checkpoint, this design cannot detect a complete
rehash.

## 4. Backup and restore evidence

```bash
memalyze backup --db ./memalyze.db --out /secure/backups/memalyze-2026-11-06.db
# Creates, without overwriting:
#   /secure/backups/memalyze-2026-11-06.db
#   /secure/backups/memalyze-2026-11-06.db.audit.json   (0600)

memalyze audit verify \
  --db /secure/backups/memalyze-2026-11-06.db \
  --checkpoint /secure/backups/memalyze-2026-11-06.db.audit.json
```

The checkpoint is computed by opening and verifying the **completed snapshot**, not by querying
the still-active source database after `VACUUM INTO`. This prevents new source writes from
pairing a snapshot with the wrong head. Keep the two files together for restore, but also copy
the checkpoint to an independent trust domain.

## 5. Optional strict audit policy

```bash
MEMALYZE_AUDIT=on MEMALYZE_AUDIT_STRICT=on ./memalyze
```

Default `off` preserves v0.1 behavior: an audit append failure is logged and the operation keeps
its normal result. With strict mode on:

- `memory_add`, `memory_delete`, and reembed return an error when their audit append fails;
- `memory_search` returns an error and clears its hits if the search cannot be audited;
- the audit write still has its own two-second deadline and uses a non-cancelled audit context.

**Strict mode is a fail-response policy, not a distributed transaction.** Add, soft-delete, and
vector writes that completed before the audit failure are not rolled back. Operators must inspect
the reported error and database state rather than assuming the mutation did not happen. Enabling
strict mode also cannot restore events that were never recorded while audit was disabled.

## 6. Static governance reports

```bash
memalyze audit report --from 2026-10-01 --to 2026-10-31 \
  --tenant acme --action add,search --format html --out governance.html
memalyze audit report --tenant acme --format json --out governance.json

# Optionally bind the report to a known point-in-time checkpoint.
memalyze audit report --checkpoint /secure/trusted.audit.json --out governance.html
```

HTML and JSON come from the same aggregate DTO: event counts by actor/action/scope, redaction
hits, redacted events, duplicates, degraded searches, filters, and full-chain verification.
Filtered aggregates are paginated internally; the verification always covers the **complete,
unfiltered** chain. Both run inside one SQLite read-only transaction, so concurrent inserts cannot
shift pagination or pair aggregates with a different chain head. A checkpoint mismatch still
writes the report for forensic inspection and returns a nonzero exit code.

The HTML is a local static document: inline CSS, no JavaScript, CDN, server, external resource,
or embedded raw-event JSON. Neither format includes memory content, memory IDs, query plaintext,
query hashes, query lengths, or `detail`. This remains true when
`MEMALYZE_AUDIT_QUERY_PLAIN=1`. Raw `audit export` is intentionally different and may contain
those privileged fields.

Each live report already freezes its own reads in one snapshot. Generate it from a static backup
when the database and report must be retained together as long-term evidence.

## 7. Remaining limits

- The chain and triggers are tamper-evident/tamper-resistant mechanisms, **not WORM**.
- Filesystem access, encryption, checkpoint storage, retention, and off-host export remain
  operator responsibilities.
- `actor` is configuration-derived metadata, not authenticated per-user identity.
- There is no automatic remote witness, signed checkpoint, timestamp authority, retention
  policy, or interactive audit UI in v0.2.0.
