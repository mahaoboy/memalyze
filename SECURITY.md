# Security policy

## Supported versions

| Version | Supported |
|---|---|
| `main` (pre-1.0) | ✅ best effort, fixes land on `main` |
| tagged releases | latest tag only |
| older tags | ❌ |

memalyze is pre-1.0. Treat it as software under active development: pin a commit, read the
release notes, and do not assume API or storage stability between minor versions yet.

## Reporting a vulnerability

**Please do not open a public issue.** Use GitHub's private reporting:

<https://github.com/mahaoboy/memalyze/security/advisories/new>

Include: version/commit, transport (stdio or HTTP), a minimal reproduction, and the impact
you believe it has (data exposure? audit bypass? crash?). We aim to acknowledge within 72
hours and to ship a fix or a mitigation note within 14 days. Credit is given in the
advisory unless you prefer otherwise.

## Threat model (what memalyze is and is not defending against)

**In scope — these are bugs:**

- plaintext PII reaching the database, FTS index, or embeddings despite `enforce` mode;
- audit rows being modified or deleted through the application, or a read path that leaves
  no audit trail;
- bypassing redaction via a non-`memory.Service` code path;
- SQL injection through tenant/scope/id/query inputs;
- path traversal or arbitrary file read through configuration (`MEMALYZE_REDACT_RULES`);
- HTTP transport accepting requests without the configured bearer token;
- serving an off-loopback listener without a token (must fail closed);
- memory-safety/crash from malformed input in the MCP layer.

**Out of scope — known design limits, documented rather than fixed:**

- **No per-user authentication.** One bearer token per process; every caller is the same
  `actor` (`MEMALYZE_ACTOR`). If two people must not read each other's memories, run two
  processes with separate tokens, databases, and tenants.
- **TLS is not terminated by memalyze.** Put it behind a reverse proxy you control.
- **The embedding model is trusted input.** A compromised Ollama endpoint can return
  arbitrary vectors; it cannot exfiltrate stored text by itself, but it can poison retrieval.
- **No encryption at rest.** The SQLite file is as protected as the filesystem it lives on.
  Use full-disk encryption or an encrypted volume.
- **The audit log is append-only at the SQL level, not WORM.** A trigger rejects `UPDATE`;
  `DELETE` is not blocked. Anyone with the database file can remove rows. If deletions are
  in scope, export to storage whose delete permission you do not hold (S3 object lock,
  syslog) and treat the local table as a buffer.
- **Audit retention is manual.** There is no purge command yet; export and archive yourself.
- **Redaction is best-effort pattern matching.** It catches the eight built-in families and
  whatever custom rules you write. It will not catch a name, an address, or a photo of an ID
  card. Do not treat it as a compliance certification.

## Hardening checklist for a deployment

1. Run the container as `nonroot` (the image does), mount only `/data` writable.
2. Set a long random `MEMALYZE_HTTP_TOKEN`; keep it out of shell history and git.
3. Bind `127.0.0.1` unless you have a reverse proxy with TLS and its own auth in front.
4. Keep `MEMALYZE_REDACT_MODE=enforce` and `MEMALYZE_AUDIT=on`.
5. Leave `MEMALYZE_AUDIT_QUERY_PLAIN` off unless your legal team explicitly wants query text.
6. Export audit logs to append-only storage you control (S3 object lock, syslog, etc.).
7. Back up the database with a consistent snapshot (the compose file uses a named volume).
8. Review `memalyze audit export` for unexpected actors or spikes in `result_count`.
