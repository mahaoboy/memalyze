# v0.2.0 — verifiable audit and governance evidence

This release strengthens the existing local governance path. It does not add cloud dependencies,
new memory tools, OAuth, import/export of memories, multi-agent features, or model routing.

## Highlights

- **Hash-chained audit (schema v3).** Every event now has a contiguous `seq`, `prev_hash`, and
  SHA-256 `hash`. Appends are serialized with `BEGIN IMMEDIATE`; callers cannot supply chain
  fields. Both `UPDATE` and `DELETE` are rejected by database triggers.
- **Automatic v2 migration.** Existing audit rows are ordered by `(ts, id)` and backfilled into
  the chain in one transaction. Historical integrity before the migration baseline is not
  retroactively proven.
- **Independent verification.** `memalyze audit verify [--json]` recomputes the complete chain.
  Optional checkpoints compare exact event count, last sequence, head, algorithm, and schema.
- **Backup trust anchors.** `memalyze backup` now writes `<snapshot>.audit.json` with mode `0600`,
  computed from the completed snapshot rather than the live source database.
- **Optional strict failures.** `MEMALYZE_AUDIT_STRICT=on` propagates audit failures. Search
  clears hits on strict audit failure. Add/delete/reembed side effects are **not** rolled back.
- **Local governance reports.** `memalyze audit report` produces static HTML or JSON aggregates,
  with full-chain verification and no raw memory/query/detail fields. HTML has no JavaScript or
  external resources.

## Upgrade

1. Back up the existing database with your current tooling.
2. Install v0.2.0 and open the database once. Schema v2 migrates automatically to v3.
3. Verify the migrated chain and create a checkpoint:

```bash
memalyze audit verify --db ./memalyze.db --write-checkpoint ./memalyze.audit.json
memalyze audit verify --db ./memalyze.db --checkpoint ./memalyze.audit.json
```

4. Copy the checkpoint somewhere a database writer cannot modify it.

Older binaries do not support schema v3. Do not point a v0.1.0 binary at an upgraded database.

## Commands

```bash
# Internal consistency; without a checkpoint this cannot detect suffix deletion.
memalyze audit verify --db ./memalyze.db

# Exact point-in-time comparison; normal newer events also mismatch an old checkpoint.
memalyze audit verify --db ./memalyze.db --checkpoint /secure/memalyze.audit.json

# Consistent snapshot + matching checkpoint (neither target is overwritten).
memalyze backup --db ./memalyze.db --out /secure/backups/memalyze.db

# Filtered aggregates + complete unfiltered chain verification.
memalyze audit report --tenant acme --from 2026-10-01 --to 2026-10-31 \
  --format html --out governance.html
memalyze audit report --tenant acme --format json --out governance.json

# Existing raw export now includes seq, prev_hash, and hash as well.
memalyze audit export --tenant acme --format json --out audit.json
```

See [`docs/audit-verification.md`](audit-verification.md) for the canonical hash format, reason
codes, checkpoint lifecycle, restore procedure, and report privacy contract.

## Security boundaries

- This is **tamper evidence, not WORM**. Anyone able to rewrite the database and every checkpoint
  can rehash a fabricated history.
- A checkpoint is unsigned; its separate, integrity-protected storage is the trust anchor.
- A valid shorter chain without a checkpoint does not prove that no suffix was removed.
- Strict audit is not an atomic business-plus-audit transaction.
- No per-user authentication, TLS termination, encryption at rest, automatic remote witness,
  retention/purge, or interactive audit UI is added in this release.

## Verification

The v0.2.0 test suite includes a real schema-v2 fixture (including NULL/empty legacy IDs), a
fixed independent hash vector, repeat-open stability, concurrent writers, UPDATE/DELETE and
`INSERT OR REPLACE` rejection with recursive triggers disabled, payload/hash/prev-hash tampering,
middle-row deletion, tail truncation with and without a checkpoint, snapshot/checkpoint agreement,
dangling-checkpoint-symlink failure handling, CLI exit codes and JSON, single-snapshot report
pagination beyond 1000 events, privacy sentinels, HTML escaping/offline output, and strict-mode
success/failure behavior.

Release automation cross-compiles five static binaries, builds four MCPB bundles, verifies both
checksum manifests, publishes the release, then downloads the public assets and repeats checksum,
version, bundle-structure, and demo checks.

## Install

Use the standalone binaries and `SHA256SUMS`, or the platform-specific `.mcpb` bundles and
`MCPB-SHA256SUMS` attached to this release. The MCPB bundles have not yet been opened on a real
macOS/Windows desktop; use the standalone binary if desktop bundle installation is unavailable.
