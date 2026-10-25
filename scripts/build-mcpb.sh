#!/usr/bin/env sh
# Build .mcpb bundles (Claude Desktop / MCPB clients) from the release binaries.
#
#   mcpb pack/validate need the official CLI:  npm install -g @anthropic-ai/mcpb
#
# One bundle per platform: the payload is a single static binary, and the MCPB
# manifest has no "pick the right binary" mechanism, so a bundle that contained
# four architectures would still only be runnable on one of them.
set -eu

cd "$(dirname "$0")/.."

OUT="${OUT:-dist}"

# Version resolution: a tag first, then dist/VERSION written by `make release`.
# Bundles must be built from the same source as the binaries they contain, so a
# dirty/non-release version is refused instead of being silently relabelled.
if [ -z "${VERSION:-}" ]; then
  VERSION=$(git describe --tags --exact-match 2>/dev/null | sed 's/^v//' || true)
fi
if [ -z "${VERSION:-}" ] && [ -f "$OUT/VERSION" ]; then
  VERSION=$(sed 's/^v//' "$OUT/VERSION")
fi
VERSION="${VERSION#v}"
case "$VERSION" in
  ""|*[!0-9A-Za-z.+-]*)
    echo "error: no release version (checkout a tag or set VERSION=)" >&2; exit 1 ;;
esac
if ! echo "$VERSION" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+([-+][0-9A-Za-z.-]+)?$'; then
  echo "error: '$VERSION' is not a release version (dirty tree? tag a commit first)" >&2
  exit 1
fi
# Reject `git describe` output for commits after a tag (0.1.0-7-gabc1234[-dirty]).
if echo "$VERSION" | grep -Eq -- '-dirty$|-[0-9]+-g[0-9a-f]+'; then
  echo "error: '$VERSION' is a git describe string, not a release version" >&2
  echo "       check out the tag you are packaging (git checkout v0.1.0)" >&2
  exit 1
fi
MCPB="${MCPB:-mcpb}"
MANIFEST="deploy/mcpb/manifest.json"

targets="${TARGETS:-darwin/arm64 darwin/amd64 windows/amd64 linux/amd64}"

if ! command -v "$MCPB" >/dev/null 2>&1; then
  echo "error: mcpb CLI not found (try: npm install -g @anthropic-ai/mcpb)" >&2
  exit 1
fi
if [ ! -f "$MANIFEST" ]; then
  echo "error: missing $MANIFEST" >&2
  exit 1
fi

mkdir -p "$OUT"

for target in $targets; do
  os=${target%/*}
  arch=${target#*/}
  ext=""
  # MCPB uses Node's process.platform names, so GOOS=windows becomes "win32".
  plat=$os
  if [ "$os" = "windows" ]; then ext=".exe"; plat=win32; fi

  bin="$OUT/memalyze-$os-$arch$ext"
  if [ ! -f "$bin" ]; then
    echo "error: missing $bin (run: make release)" >&2
    exit 1
  fi

  stage="$OUT/mcpb/$os-$arch"
  rm -rf "$stage"
  mkdir -p "$stage/server"
  cp "$bin" "$stage/server/memalyze$ext"
  chmod 755 "$stage/server/memalyze$ext"

  sed -e "s/__VERSION__/$VERSION/g" \
      -e "s/__PLATFORM__/$plat/g" \
      -e "s#__ENTRY__#server/memalyze$ext#g" \
      "$MANIFEST" > "$stage/manifest.json"

  echo "== $os/$arch"
  if ! "$MCPB" validate "$stage/manifest.json" >/dev/null; then
    echo "error: manifest validation failed for $os/$arch" >&2
    exit 1
  fi
  out="$OUT/memalyze-$VERSION-$os-$arch.mcpb"
  "$MCPB" pack "$stage" "$out" >/dev/null
  echo "   $out  $(du -h "$out" | cut -f1)"
done

# Checksums for the registry's fileSha256 fields and for humans who download by hand.
( cd "$OUT" && (shasum -a 256 memalyze-"$VERSION"-*.mcpb 2>/dev/null || sha256sum memalyze-"$VERSION"-*.mcpb) > MCPB-SHA256SUMS )
echo
cat "$OUT/MCPB-SHA256SUMS"
