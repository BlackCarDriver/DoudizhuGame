#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
PID=shell/doudizhu.pid
if [ ! -f "$PID" ]; then
  echo "没有 pid 文件"
  exit 0
fi
PIDV="$(cat "$PID")"
if kill -0 "$PIDV" 2>/dev/null; then
  kill "$PIDV"
  for _ in $(seq 1 30); do
    if ! kill -0 "$PIDV" 2>/dev/null; then break; fi
    sleep 0.1
  done
  echo "已停止（pid ${PIDV}）"
else
  echo "进程不存在（pid ${PIDV}）"
fi
rm -f "$PID"
