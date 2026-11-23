#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p shell data
PID=shell/doudizhu.pid
LOG=shell/doudizhu.log
if [ -f "$PID" ] && kill -0 "$(cat "$PID")" 2>/dev/null; then
  echo "已在运行（pid $(cat "$PID")）"
  exit 0
fi
nohup ./doudizhu "$@" >> "$LOG" 2>&1 &
echo $! > "$PID"
echo "已启动（pid $(cat "$PID")），日志 $LOG"
