# Contributing to memalyze

Thanks for looking. This is a small, opinionated project; the fastest way to get a change
merged is to keep it small and to respect the constraints below.

## The three constraints that shape every decision

1. **Tool surface stays small.** Four tools today (`memory_add`, `memory_search`,
   `memory_delete`, `echo`), hard ceiling five. A new tool needs a written justification:
   why the existing tools cannot express the operation, and what it costs the model in
   selection accuracy.
2. **One static binary, fully offline.** No CGO, no runtime model download, no external
   service required for the default path. A new dependency must be pure Go and must not
   open a network connection on startup.
3. **Governance is not optional.** Anything that writes memory must pass redaction first,
   and anything that reads memory must be auditable. A feature that bypasses
   `memory.Service` (for example a raw SQL path in the MCP layer) will be rejected.

## Development

```bash
make build          # CGO_ENABLED=0
make test           # go test ./...
go vet ./...
gofmt -l ./cmd ./internal ./pkg   # must print nothing
make demo           # the 30-second governance demo
```

Optional, needs Ollama with `qwen3-embedding:0.6b`:

```bash
MEMALYZE_IT=1 go test ./internal/memory/ ./internal/store/sqlite/ -run Integration
```

Tests must not require network access. Integration tests are skipped unless `MEMALYZE_IT=1`.

## What a good PR looks like

- One logical change per PR; the title says which step it serves (`feat(S15): …`).
- New behaviour comes with a test that would fail without it. Bug fixes come with the
  reproduction.
- User-visible changes update `README.md` or `docs/`; architecture decisions update
  `docs/` too (the project keeps a decision log outside this repo).
- No drive-by reformatting; `gofmt` only.
- If you change semantics (dedup, degradation, redaction position), say so explicitly in the
  PR description — those are contract-level changes.

## Commit messages

`type(scope): summary` with types `feat`, `fix`, `test`, `docs`, `refactor`, `chore`.
Chinese or English is fine; be specific about *why*.

## Licensing of contributions

memalyze is **AGPL-3.0-or-later** and is also offered under a **commercial license** to
companies that cannot use AGPL. By submitting a pull request you confirm that:

- you wrote the contribution or have the right to submit it, and
- you license it to the project under AGPL-3.0-or-later, **and**
- you grant the maintainer the right to relicense your contribution under the commercial
  license.

If that is not acceptable for your employer, open an issue first — we can discuss a
different arrangement before you write code.

## Security

Do **not** open a public issue for a vulnerability. See [`SECURITY.md`](SECURITY.md).

## Ideas that are explicitly out of scope

- A web UI for memory browsing.
- Reversible redaction or key management (v0.1 stores no keys).
- Graph/entity extraction pipelines.
- Hosted SaaS, telemetry on by default, or anything that phones home.
