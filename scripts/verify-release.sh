#!/usr/bin/env bash
# 发布后核验：Release 是否公开、资产是否齐全、SHA256SUMS 是否对得上。
# 只依赖 api.github.com 与 objects.githubusercontent.com（不经过 github.com 网页）。
#
#   ./scripts/verify-release.sh v0.1.0
set -euo pipefail

REPO="${REPO:-mahaoboy/memalyze}"
TAG="${1:-v0.1.0}"
API="https://api.github.com/repos/${REPO}/releases/tags/${TAG}"

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

echo "== release ${TAG} =="
if ! curl -fsSL -m 20 "$API" -o "$tmp/release.json"; then
  echo "FAIL: ${TAG} 不是公开 Release（草稿或未创建）" >&2
  exit 1
fi

python3 - "$tmp/release.json" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
print(f"name:      {r['name']}")
print(f"draft:     {r['draft']}  prerelease: {r['prerelease']}")
print(f"published: {r['published_at']}")
print(f"assets:    {len(r['assets'])}")
for a in r["assets"]:
    print(f"  - {a['name']:<28} {a['size']/1048576:6.1f} MB  downloads={a['download_count']}")
PY

echo "== download =="
python3 - "$tmp/release.json" > "$tmp/urls.txt" <<'PY'
import json, sys
r = json.load(open(sys.argv[1]))
for a in r["assets"]:
    print(a["name"], a["url"])
PY

while read -r name url; do
  curl -fsSL -m 180 -H "Accept: application/octet-stream" "$url" -o "$tmp/$name"
  echo "  ok $name"
done < "$tmp/urls.txt"

echo "== checksums =="
if [ -f "$tmp/SHA256SUMS" ]; then
  ( cd "$tmp" && sha256sum -c SHA256SUMS )
else
  echo "FAIL: SHA256SUMS missing" >&2
  exit 1
fi

echo "== smoke test (linux-amd64) =="
if [ -f "$tmp/memalyze-linux-amd64" ]; then
  chmod +x "$tmp/memalyze-linux-amd64"
  "$tmp/memalyze-linux-amd64" --version
  "$tmp/memalyze-linux-amd64" demo >/dev/null && echo "  demo: ok"
fi

echo "PASS: ${TAG} is public, assets verified"
