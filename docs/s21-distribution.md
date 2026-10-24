# S21 — Packaging and distribution

- **Status**: artifacts ready (2026-10-25); external uploads pending (see §5)
- **Milestone**: M4 / S21
- **Acceptance**: a stranger can get from zero to a working MCP server in three steps;
  the project is discoverable in at least one directory.

## 1. What ships

| Artifact | How it is produced | Verified |
|---|---|---|
| 5 static binaries (linux/darwin amd64+arm64, windows amd64) | `make release` | ✅ built locally, 13–14 MB each |
| `SHA256SUMS` | `make release` | ✅ |
| GitHub Release (draft) with notes | `.github/workflows/release.yml` on `v*` tag | ⚠️ workflow not yet run |
| `scripts/install.sh` (curl \| sh, checksum-verified) | hand-written | ✅ syntax checked; end-to-end needs a published release |
| dsh / agent skill | `skills/memalyze/` | ✅ content complete |
| Docker image | `Dockerfile` + compose | ❌ daemon unavailable (B5) |
| MCP directory listing | pending submission | ❌ |
| ClawHub listing | pending submission | ❌ |

## 2. Three-step install (the acceptance test)

```bash
./scripts/install.sh                        # 1. binary (+ checksum)
ollama pull qwen3-embedding:0.6b            # 2. model
claude mcp add --scope project memalyze -- memalyze   # 3. wire it up
```

Anything that adds a fourth step (editing JSON by hand, building from source, starting a
database) fails this criterion and must be fixed before announcing.

## 3. Versioning

- `v0.1.0` is the first public tag. Pre-1.0: minor bumps may break storage or tool output;
  say so in the release notes.
- Tag format `vMAJOR.MINOR.PATCH`; the binary embeds the tag via
  `-ldflags "-X main.version=..."` (already wired in the Makefile).
- Release notes are generated from commits, then edited: what broke, what to do about it,
  and the upgrade path for existing databases (schema migrations run automatically).

## 4. Channels, in priority order

| Channel | Why | Cost | Status |
|---|---|---|---|
| GitHub Releases + README | the canonical artifact and the thing people link to | 0 | ready |
| MCP server directories (mcporter / awesome-mcp list) | where MCP users actually look | 30 min PR | pending |
| dsh skill catalog (`skills/memalyze`) | agent-native install; matches how the tool is used | 30 min | ready to submit |
| ClawHub | same audience as dsh, low effort | 30 min | pending |
| ChatGPT plugin market | 12B weekly users, but likely requires overseas entity + hosted backend | high | **deliberately deferred** (O-009) |
| HN / Reddit / V2EX | first feedback, not first users | 2 h | copy in `docs/s22-launch.md` |

## 5. What is blocked, and on what

| Item | Blocker | Unblock |
|---|---|---|
| Docker image build | no Docker daemon in the dev environment (B5) | `sudo systemctl start docker`, then `make docker-build` |
| GitHub Release | needs a pushed tag and CI minutes | `git tag v0.1.0 && git push origin v0.1.0` |
| installer end-to-end | needs a published release with `SHA256SUMS` | same tag |
| directory submissions | external review, account-bound | manual, one evening |

## 6. Post-release checklist (first 72 hours)

- [ ] `curl -fsSL .../install.sh | sh` on a clean machine (no Go installed)
- [ ] install with `VERSION=v0.1.0` pinned, not just `latest`
- [ ] `memalyze demo` passes on that machine (no Ollama, no network)
- [ ] README's three steps work verbatim
- [ ] `memalyze audit export` on the demo database produces a non-empty CSV
- [ ] release assets have checksums that match `SHA256SUMS`
- [ ] watch issues for install failures → they are R1 signals, not noise
