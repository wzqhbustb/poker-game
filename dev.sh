#!/usr/bin/env bash
# 一键启动本地开发环境：后端 pokerd(:8080) + 前端 Vite(:5173)。
# 用法: ./dev.sh [-fast]
#   -fast 传给 pokerd，bot 不模拟思考延时（调试用）
set -euo pipefail
cd "$(dirname "$0")"

POKERD_ARGS=()
for arg in "$@"; do
  POKERD_ARGS+=("$arg")
done

cleanup() {
  kill 0 2>/dev/null || true
}
trap cleanup EXIT INT TERM

(cd server && go run ./cmd/pokerd -addr :8080 -db data/poker.db -web ../web/dist "${POKERD_ARGS[@]}") &
(cd web && npm run dev) &

echo "后端  http://localhost:8080"
echo "前端  http://localhost:5173  (浏览器打开这个)"
wait
