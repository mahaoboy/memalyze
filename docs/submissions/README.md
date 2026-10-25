# Directory & registry submissions

Everything here is prepared so the remaining work is copy/paste plus a login, not writing.
Update the tracking table at the bottom after each submission.

Last checked upstream: **2026-10-25**.

| Channel | Artifact | Blocked on |
|---|---|---|
| MCP Registry (`registry.modelcontextprotocol.io`) | [`../../deploy/registry/server.json`](../../deploy/registry/server.json) | ✅ published 2026-10-25 (`io.github.mahaoboy/memalyze@0.1.0`) |
| `punkpeye/awesome-mcp-servers` | [`awesome-mcp-servers.patch`](awesome-mcp-servers.patch) + [PR text](awesome-mcp-servers-pr.md) | ✅ PR [#17582](https://github.com/punkpeye/awesome-mcp-servers/pull/17582) open |
| `modelcontextprotocol/servers` | — | **do not submit** (see §3) |
| ClawHub / dsh skill catalog | `make skill` → `dist/memalyze-skill-<version>.tar.gz` | your account |

---

## 1. MCP Registry — via MCPB, no Docker needed

The registry only stores **metadata**; the artifact stays on GitHub Releases. MCPB is an
officially supported package type, which is why this path does not need the Docker image
(B5) at all.

Order matters:

```sh
# 1. Make sure server.json still matches the files you are about to upload.
make verify-mcpb

# 2. The server.json URLs must exist before publishing, or validation/publish fails.
gh release upload v0.1.0 \
  dist/memalyze-0.1.0-darwin-arm64.mcpb \
  dist/memalyze-0.1.0-darwin-amd64.mcpb \
  dist/memalyze-0.1.0-windows-amd64.mcpb \
  dist/memalyze-0.1.0-linux-amd64.mcpb \
  dist/MCPB-SHA256SUMS

# 3. CLI (macOS/Linux; see the upstream quickstart for Windows)
curl -L "https://github.com/modelcontextprotocol/registry/releases/latest/download/mcp-publisher_$(uname -s | tr '[:upper:]' '[:lower:]')_$(uname -m | sed 's/x86_64/amd64/;s/aarch64/arm64/').tar.gz" | tar xz mcp-publisher
sudo mv mcp-publisher /usr/local/bin/

# 4. Validate, then publish
#    (locally we validated the same file against the official 2025-12-11 JSON Schema;
#     the mcp-publisher binary download timed out from this host, so run this step yourself)
mcp-publisher validate deploy/registry/server.json
mcp-publisher login github          # device flow, needs your GitHub account
mcp-publisher publish
```

Facts this file depends on (checked 2026-10-25):

- `registryType: "mcpb"` accepts GitHub/GitLab **release** URLs only, and `fileSha256` is
  mandatory for MCPB. The registry does not verify the hash, clients do.
- The package URL must contain the string `mcp` — `.mcpb` satisfies it.
- Namespace `io.github.mahaoboy` is proven by logging in with the GitHub account that owns
  the repo.
- One bundle per platform, because the MCPB manifest has no "pick the right binary" field.
  The `packages` array therefore has four entries. **This shape is untested against the live
  registry**; if publish rejects duplicate `mcpb` entries, keep the `darwin-arm64` one and
  point the others at the release page.

For a **future** release, `server.json` must be generated from the uploaded bytes (packing is
not byte-reproducible):

```sh
python3 scripts/gen-server-json.py --from-release v0.1.1
git commit deploy/registry/server.json -m "chore(registry): v0.1.1"
```

Verify after publishing:

```sh
curl -s 'https://registry.modelcontextprotocol.io/v0/servers?search=memalyze' | jq '.servers[].server | {name, version, packages}'
```

Then check the MCPB itself on a Mac/Windows machine (B8): download, open with Claude Desktop,
confirm the install sheet appears, `memory_add` works, and the DB lands where `user_config`
says. Record the result in the tracking table — untested bundles should not be advertised.

## 2. awesome-mcp-servers

Ready-to-apply patch against README blob `539d1b0e02…` (2026-10-25). It adds one line at the
end of **Knowledge & Memory**:

```sh
git clone https://github.com/<you>/awesome-mcp-servers.git
cd awesome-mcp-servers && git checkout -b add-memalyze
git apply /path/to/memalyze/docs/submissions/awesome-mcp-servers.patch
git add README.md && git commit -m "Add memalyze (self-hosted memory with redaction + audit)"
git push origin add-memalyze
```

Title and body: [`awesome-mcp-servers-pr.md`](awesome-mcp-servers-pr.md). If `main` has moved,
re-fetch `README.md` and regenerate rather than forcing the patch: it has 3 lines of context
and should fail loudly.

## 3. modelcontextprotocol/servers — do not submit

Two independent reasons, both checked 2026-10-25:

1. Their `CONTRIBUTING.md` states: **"We accept issues, not pull requests."** PRs from
   non-maintainers are closed with a pointer to that document.
2. The README now says the repo "is dedicated to housing just the small number of reference
   servers maintained by the MCP steering group" and sends directory traffic to the registry.

So the earlier plan (S21) to PR a community entry there is obsolete. If there is something
worth reporting, open an **issue**, not a PR.

## 4. ClawHub / dsh skill catalog

```sh
make skill     # dist/memalyze-skill-<version>.tar.gz
```

Submission fields: name `memalyze`; one-liner "persistent, self-hosted memory for agents —
redaction before write, audit on read"; install
`curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh`;
requires Ollama + `qwen3-embedding:0.6b`; homepage the repo.

## 5. Tracking

| Channel | Submitted | Link | Outcome |
|---|---|---|---|
| MCP Registry | 2026-10-25 | [API entry](https://registry.modelcontextprotocol.io/v0/servers?search=memalyze) | ✅ published, 4 mcpb packages accepted |
| awesome-mcp-servers | 2026-10-25 | [#17582](https://github.com/punkpeye/awesome-mcp-servers/pull/17582) | 🟡 open, awaiting review |
| modelcontextprotocol/servers | n/a | — | not accepted (issues only) |
| ClawHub | — | — | skill bundle ready |
