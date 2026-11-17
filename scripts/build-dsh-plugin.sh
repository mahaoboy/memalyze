#!/usr/bin/env sh
# Build the DSH plugin tarball: five static binaries, gzip them, npm pack.
#
#   ./scripts/build-dsh-plugin.sh
#   VERSION=0.2.0 ./scripts/build-dsh-plugin.sh
#
# The binaries are deliberately not committed: ~70 MB raw / ~28 MB gzipped.
# npm carries them; a source checkout builds them with this script.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
PKG="$ROOT/integrations/dsh"
BIN="$PKG/bin"
VERSION="${VERSION:-$(node -p "require('$PKG/package.json').version")}"

mkdir -p "$BIN" "$ROOT/dist"
rm -f "$BIN"/memalyze-* "$BIN"/*.gz

for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  os=${target%/*}
  arch=${target#*/}
  ext=""
  [ "$os" = "windows" ] && ext=".exe"
  asset="memalyze-$os-$arch$ext"
  echo "building $os/$arch (version $VERSION)"
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build \
    -trimpath -ldflags "-s -w -X main.version=$VERSION" \
    -o "$BIN/$asset" "$ROOT/cmd/memalyze"
  gzip -9 -c "$BIN/$asset" > "$BIN/$asset.gz"
  rm -f "$BIN/$asset"
done

echo "packaging dsh-plugin-memalyze@$VERSION"
(
  cd "$PKG"
  npm pack --pack-destination "$ROOT/dist"
)

echo
ls -lh "$BIN" "$ROOT/dist/dsh-plugin-memalyze-$VERSION.tgz"
echo
echo "verify locally:"
echo "  node $PKG/test/setup-plugin.test.mjs"
echo "  node $PKG/test/mcp-smoke.mjs"
