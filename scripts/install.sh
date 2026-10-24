#!/usr/bin/env sh
# memalyze installer: download a prebuilt static binary and verify its checksum.
#
#   curl -fsSL https://raw.githubusercontent.com/mahaoboy/memalyze/main/scripts/install.sh | sh
#
# Environment:
#   VERSION   release tag to install (default: latest)
#   BIN_DIR   install directory (default: $HOME/.local/bin)
set -eu

REPO="${REPO:-mahaoboy/memalyze}"
VERSION="${VERSION:-latest}"
BIN_DIR="${BIN_DIR:-$HOME/.local/bin}"

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) echo "unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  *) echo "unsupported OS: $os (use the Windows .exe from the release page)" >&2; exit 1 ;;
esac

asset="memalyze-${os}-${arch}"
if [ "$VERSION" = "latest" ]; then
  base="https://github.com/${REPO}/releases/latest/download"
else
  base="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fetch() {
  if command -v curl >/dev/null 2>&1; then curl -fsSL "$1" -o "$2"
  elif command -v wget >/dev/null 2>&1; then wget -qO "$2" "$1"
  else echo "need curl or wget" >&2; exit 1
  fi
}

echo "downloading ${asset} (${VERSION})"
fetch "${base}/${asset}" "${tmp}/${asset}"

# Checksum verification is not optional: it is the only integrity check a curl|sh install has.
if fetch "${base}/SHA256SUMS" "${tmp}/SHA256SUMS" 2>/dev/null; then
  expected=$(grep " ${asset}\$" "${tmp}/SHA256SUMS" | awk '{print $1}' | head -n1)
  if [ -z "$expected" ]; then
    echo "checksum for ${asset} missing from SHA256SUMS; refusing to install" >&2
    exit 1
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    actual=$(sha256sum "${tmp}/${asset}" | awk '{print $1}')
  else
    actual=$(shasum -a 256 "${tmp}/${asset}" | awk '{print $1}')
  fi
  if [ "$expected" != "$actual" ]; then
    echo "checksum mismatch for ${asset}: expected ${expected}, got ${actual}" >&2
    exit 1
  fi
  echo "checksum ok"
else
  echo "warning: SHA256SUMS not published for ${VERSION}; skipping integrity check" >&2
fi

mkdir -p "$BIN_DIR"
chmod +x "${tmp}/${asset}"
mv "${tmp}/${asset}" "${BIN_DIR}/memalyze"
echo "installed ${BIN_DIR}/memalyze"
"${BIN_DIR}/memalyze" --version || true

cat <<'MSG'

next steps:
  1. ollama pull qwen3-embedding:0.6b
  2. claude mcp add --scope project memalyze -- memalyze
  3. memalyze demo          # 30-second redaction + audit demo
MSG
