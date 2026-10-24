# memalyze behind Docker (HTTP transport)

Use this when memalyze must outlive a single harness process: a shared team instance, a
server, or a demo box. For a laptop with one harness, stdio is simpler.

```bash
# 1. token + stack
printf 'MEMALYZE_HTTP_TOKEN=%s\n' "$(openssl rand -hex 24)" > .env
export MEMALYZE_HTTP_TOKEN="$(grep MEMALYZE_HTTP_TOKEN .env | cut -d= -f2)"
docker compose up -d --build

# 2. verify
curl -s http://127.0.0.1:8080/healthz          # {"status":"ok","version":"dev"}

# 3. point Claude Code at it
claude mcp add --transport http memalyze http://127.0.0.1:8080/mcp \
  --header "Authorization: Bearer $MEMALYZE_HTTP_TOKEN"
```

Other clients can copy [`mcp-http.json`](mcp-http.json) (substitute the token).

## What changes compared with stdio

| | stdio | HTTP |
|---|---|---|
| Lifetime | spawned per harness session | long-running service |
| Isolation | one process per client | one process, many sessions |
| Auth | OS process boundary | **bearer token required off-loopback** |
| Actor in audit | `MEMALYZE_ACTOR` per process | one actor for the whole container — set it to the team name |
| Data | `./memalyze.db` | named volume `memalyze-data` |

Because there is no per-user auth yet, **one token = one trust boundary**. If two teams
must not read each other's memories, run two containers with different tokens and volumes
until v0.2 lands auth.

## Backups

The database is a single file in the `memalyze-data` volume:

```bash
docker compose exec memalyze true       # container is distroless: no shell
docker run --rm -v memalyze_memalyze-data:/data -v "$PWD":/backup alpine \
  tar czf /backup/memalyze-$(date +%F).tar.gz -C /data .
```

Audit export works without stopping the server:

```bash
docker compose run --rm memalyze audit export --db /data/memalyze.db --format csv
```
