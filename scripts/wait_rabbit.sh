#!/usr/bin/env bash
set -euo pipefail
HOST="${1:-localhost}"
PORT="${2:-5672}"
TIMEOUT="${3:-30}"
START=$(date +%s)
while true; do
  if nc -z "$HOST" "$PORT" 2>/dev/null; then
    echo "[wait_rabbit] $HOST:$PORT is up"
    exit 0
  fi
  NOW=$(date +%s)
  if (( NOW - START > TIMEOUT )); then
    echo "[wait_rabbit] timeout waiting for $HOST:$PORT" >&2
    exit 1
  fi
  sleep 1
done