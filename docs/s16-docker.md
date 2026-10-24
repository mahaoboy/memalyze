# S16 — Docker: five minutes to a governed memory server

- **Status**: files done (2026-10-25); **image build not executed here** (see §6)
- **Milestone**: M3 / S16
- **Scope**: Dockerfile, compose (memalyze + Ollama + one-shot model pull), HTTP transport
  with a mandatory token off-loopback, container healthcheck.

## 1. Why HTTP transport had to land with Docker

Before S16 memalyze only spoke MCP over **stdio**. That is fine when a harness spawns the
binary, but inside a container stdio is close to useless: `docker compose up` would start a
process that nothing can talk to.

So S16 adds a second transport — MCP **Streamable HTTP** on `/mcp`
(`memalyze --http :8080`, or `MEMALYZE_HTTP_ADDR`) — plus `/healthz`. Both transports share
the same `mcp.NewServer` and the same `memory.Service`, so governance semantics (redaction,
audit) are identical.

## 2. Quick start

```bash
printf 'MEMALYZE_HTTP_TOKEN=%s\n' "$(openssl rand -hex 24)" > .env
docker compose up -d --build          # ollama + model pull + memalyze
curl -s http://127.0.0.1:8080/healthz # {"status":"ok","version":"dev"}
```

The first run downloads `qwen3-embedding:0.6b` (~640 MB) into the `ollama` volume via the
one-shot `model-pull` service. After that the stack is fully offline.

Point any Streamable-HTTP MCP client at `http://127.0.0.1:8080/mcp` with
`Authorization: Bearer $MEMALYZE_HTTP_TOKEN`.

## 3. What is in the image

- Multi-stage build: `golang:1.27-alpine` → `gcr.io/distroless/static-debian12:nonroot`.
- `CGO_ENABLED=0` + `-trimpath` + `-ldflags "-s -w"`: static, reproducible-ish, ~14 MB.
- No shell, no package manager, runs as `nonroot`, only `/data` is writable.
- Healthcheck uses the binary itself (`memalyze health --url .../healthz`) because distroless
  has no `curl`.

## 4. Security defaults (and why they are strict)

| Default | Reason |
|---|---|
| `MEMALYZE_HTTP_TOKEN` is **required** by compose (`:?` interpolation) | a compose file that starts an unauthenticated memory store is a footgun |
| Binary refuses to bind a non-loopback address without a token | fail closed at startup, not at the first request |
| Compose publishes `127.0.0.1:8080:8080` only | exposing to the LAN must be an explicit edit |
| Token compared with `subtle.ConstantTimeCompare` | avoid trivial timing oracles |
| Redaction `enforce` + audit `on` in the image env | the container should be the safe configuration by default |

What this is **not**: multi-user auth. There is one token per process, and `actor` comes
from `MEMALYZE_ACTOR`. Run one container per trust boundary until v0.2 adds per-user auth.

## 5. Files

| File | Purpose |
|---|---|
| `Dockerfile` | two-stage static build, distroless runtime |
| `docker-compose.yml` | ollama + model-pull + memalyze, healthchecks, volumes |
| `.dockerignore` | keeps `.git`, DBs and the local binary out of the context |
| `.env.example` | the four values you actually set |
| `Makefile` | `build` / `test` / `demo` / `docker-build` / `docker-up` / `release` |

## 6. Verification status

Verified in this environment:

- `make build` → static binary, `./memalyze --version` OK;
- `make release` → **5/5 cross-compiled targets** (linux/darwin amd64+arm64, windows amd64)
  with `SHA256SUMS`;
- HTTP smoke test: `/healthz` → `{"status":"ok"}`, `memalyze health` → `ok`,
  `POST /mcp` without a token → `401`, and the unauthenticated-public-bind guard covered by
  unit tests.

**Not verified**: `docker build` / `docker compose up`. The daemon is not running in this
environment (B5: `dial unix /var/run/docker.sock: no such file or directory`, and `sudo`
is blocked by `no_new_privs`). The Dockerfile is therefore "written and reviewed" rather
than "built". First action when a daemon is available:

```bash
make docker-build && make docker-up && curl -s http://127.0.0.1:8080/healthz
```

Recorded as B5 in `00-总览/继续点.md`.

## 7. Troubleshooting

| Symptom | Cause | Fix |
|---|---|---|
| `refusing to listen on ":8080" without MEMALYZE_HTTP_TOKEN` | public bind without token | set the token, or bind `127.0.0.1` |
| `401 unauthorized` from a client | wrong/missing `Authorization: Bearer` | reuse the value in `.env` |
| `/healthz` OK but search always degraded | Ollama not reachable or model not pulled | `docker compose logs model-pull`, then `docker compose exec ollama ollama list` |
| First query slow (~40 s) then fine | embedding cold load; warmup still running | wait for `embedding warmup ok` in logs (D-018) |
