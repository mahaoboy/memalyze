## What and why

<!-- One paragraph. Link the issue if there is one. -->

## Which constraint does this touch?

- [ ] Tool surface (still ≤5 tools; no new tool unless justified below)
- [ ] Offline / single binary (no CGO, no runtime download, no new service)
- [ ] Governance (redaction before write, audit on read)

## Checks

- [ ] `gofmt -l ./cmd ./internal ./pkg` prints nothing
- [ ] `go vet ./...` passes
- [ ] `go test ./...` passes, and new behaviour has a test that fails without it
- [ ] Docs updated (`README.md` or `docs/`) for user-visible changes
- [ ] No real customer data, tokens, or memory contents in tests/fixtures

## Semantics change?

<!-- Dedup, degradation, redaction position, audit fields — call it out explicitly. -->
