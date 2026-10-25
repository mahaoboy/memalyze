# S21 — Packaging and distribution

- **Status**: **v0.1.0 published** (2026-10-25); `.mcpb` bundles built and validated, not yet uploaded; directory submissions still pending (see §5)
- **Milestone**: M4 / S21
- **Acceptance**: a stranger can get from zero to a working MCP server in three steps;
  the project is discoverable in at least one directory.

## 1. What ships

| Artifact | How it is produced | Verified |
|---|---|---|
| 5 static binaries (linux/darwin amd64+arm64, windows amd64) | `make release` | ✅ built locally, 13–14 MB each; **rebuilding tag `v0.1.0` reproduces the published SHA-256 exactly** |
| `SHA256SUMS` | `make release` | ✅ |
| GitHub Release with notes | `.github/workflows/release.yml` on `v*` tag | ✅ **published**, 6 assets, checksums verified |
| `scripts/install.sh` (curl \| sh, checksum-verified) | hand-written | ⚠️ end-to-end run from this host is slow (13.7 MB in 76 s, 174 KB/s); manual download + `sha256sum -c` verified, see §7 |
| dsh / agent skill | `skills/memalyze/` | ✅ content complete |
| `.mcpb` bundles (Claude Desktop / MCPB clients) | `make mcpb` | ✅ 4 platforms, `mcpb validate` + `pack` pass; binary inside reports `v0.1.0`; **not yet uploaded**, not yet opened in Claude Desktop (B8) |
| `deploy/registry/server.json` | `scripts/gen-server-json.py` | ✅ valid against the official `2025-12-11` schema; publish blocked on upload + login (B7) |
| Docker image | `Dockerfile` + compose | ❌ daemon unavailable (B5) |
| MCP directory listing | [`submissions/`](submissions/README.md): MCPB route (no Docker) + awesome patch | ❌ not submitted (B7) |
| ClawHub listing | `make skill` produces the package | ❌ not submitted |

## 2. Three-step install (the acceptance test)

```bash
./scripts/install.sh                        # 1. binary (+ checksum)
ollama pull qwen3-embedding:0.6b            # 2. model
claude mcp add --scope project memalyze -- memalyze   # 3. wire it up
```

Anything that adds a fourth step (editing JSON by hand, building from source, starting a
database) fails this criterion and must be fixed before announcing.

## 2.1 Publishing is automated

Since `06fb2c1` + the workflow update on 2026-10-25, `.github/workflows/release.yml`:

1. runs the full test suite,
2. cross-compiles and **verifies `SHA256SUMS` against its own artifacts** (the v0.1.0 bug),
3. publishes a **non-draft** release with auto-generated notes,
4. re-downloads every asset from the public API and runs `--version` + `demo`
   (`scripts/verify-release.sh`).

So `git tag v0.1.1 && git push origin v0.1.1` is the whole release. No manual publish click.

## 2.2 MCPB bundles and the registry

`make mcpb` does three things, in this order:

1. cross-compiles the five release binaries (`make release`), writing `dist/VERSION`;
2. stages one bundle per platform and runs `mcpb validate` + `mcpb pack`
   (`scripts/build-mcpb.sh`);
3. derives `fileSha256` for every bundle and rewrites `deploy/registry/server.json`
   (`scripts/gen-server-json.py`) — hashes are never typed by hand.

Two rules that came out of the first build:

- **Build from a clean tag, not from `HEAD`.** The first attempt packaged binaries that were
  four commits past `v0.1.0` (and dirty) under a `0.1.0` label. The script now rejects
  `git describe` strings like `0.1.0-7-gabc1234-dirty`, and also refuses any binary whose
  `go version -m` says `vcs.modified=true` — Go stamps VCS state into the binary, so a dirty
  tree changes the bytes even when `VERSION` is overridden to look clean.
- **`mcpb pack` is not byte-reproducible** (timestamps), so `server.json` and
  `MCPB-SHA256SUMS` must be regenerated after every pack, and the uploaded bytes are the
  ones that were hashed. The *binaries* inside are reproducible: rebuilding tag `v0.1.0` from a clean
  checkout matched all five published hashes, including VCS stamps.

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
| ~~GitHub Release~~ | done: `v0.1.0` published 2026-10-24T18:10Z | — |
| `.mcpb` upload + registry publish | assets must exist before `server.json` URLs work; `mcp-publisher login github` needs your account (B7) | `gh release upload ...` then `mcp-publisher publish` (see `submissions/README.md`) |
| `.mcpb` on a real desktop | no macOS/Windows machine here (B8) | open the bundle in Claude Desktop, run `memory_add`/`memory_search` |
| installer end-to-end on this host | reachable but slow: 13.7 MB took 76 s (174 KB/s), HEAD requests ~20 s through two redirects | run it from a normal network; for CN users mention the size and the release page (§7) |
| directory submissions | external review, account-bound | manual, one evening |

## 5.1 What publishing actually caught

The first publish exposed a real bug in `make release`: `(shasum -a 256 * …) > SHA256SUMS`
creates `SHA256SUMS` before the glob expands, so the file hashed **itself as an empty file**.
Every downloader's `sha256sum -c` would fail on that one line.

Fixed in `06fb2c1` (hash only `memalyze-*`), the published asset was recomputed from the
already-uploaded binaries and replaced, and `scripts/verify-release.sh` now re-downloads all
six assets, checks every checksum, runs `--version` and `demo` before reporting PASS.

## 6. Post-release checklist (first 72 hours)

- [ ] `curl -fsSL .../install.sh | sh` on a clean machine (no Go installed)
- [ ] install with `VERSION=v0.1.0` pinned, not just `latest`
- [ ] `memalyze demo` passes on that machine (no Ollama, no network)
- [ ] README's three steps work verbatim
- [ ] `memalyze audit export` on the demo database produces a non-empty CSV
- [ ] release assets have checksums that match `SHA256SUMS`
- [ ] watch issues for install failures → they are R1 signals, not noise
- [ ] open the `.mcpb` in Claude Desktop on macOS and Windows; confirm `memory_add` works and
      the DB lands at `user_config.db_path`
- [ ] after registry publish, `curl '…/v0/servers?search=memalyze'` returns the entry

## 7. Slow-network reality (measured 2026-10-25, CN residential line)

| Action | Result |
|---|---|
| `HEAD` on the release asset URL | ~20 s (two 302s, ends at `release-assets.githubusercontent.com`) |
| full 13.7 MB `memalyze-linux-amd64` download | 76 s → **174 KB/s** |
| `api.github.com` | fast (< 1 s) |
| `raw.githubusercontent.com`, `cdn.jsdelivr.net` | unreachable from this host |

So `curl \| sh` works but is not instant for users on a similar line. Do not hide it: the
V2EX draft says the size and points at the release page. A mirror is not set up — do not
claim one exists.
