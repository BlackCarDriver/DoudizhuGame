#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."

# 1) 构建前端（产物在 web/dist，Go 直接托管）
if [ -d ../web ]; then
  (cd ../web && npm run build)
fi

# 2) 构建 Linux 二进制（阿里服务器没有 Go，必须本地交叉编译）
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o doudizhu .
echo "已生成 $(pwd)/doudizhu"
