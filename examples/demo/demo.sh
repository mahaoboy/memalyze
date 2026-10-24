#!/usr/bin/env bash
# 30 秒企业治理演示：脱敏在写入前 + 读取有审计。
#
# 不需要 Ollama、不需要 MCP 客户端、不需要联网：
#   ./examples/demo/demo.sh
#
# 演示会打印「数据库里真正存的内容」和「审计导出」，可直接投屏给客户看。
set -euo pipefail

cd "$(dirname "$0")/../.."

GO="${GO:-go}"
if ! command -v "$GO" >/dev/null 2>&1; then
  if [ -x "$HOME/.local/go/bin/go" ]; then
    GO="$HOME/.local/go/bin/go"
  else
    echo "go not found; install Go 1.25+ or set GO=/path/to/go" >&2
    exit 1
  fi
fi

# 演示数据落在临时目录，跑完即删（--keep 可保留，见 memalyze demo -h）
exec "$GO" run ./cmd/memalyze demo "$@"
