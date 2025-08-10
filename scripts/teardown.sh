#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

echo "[teardown] Stopping web server and demo scripts..."
# Stop FastAPI server and demo python processes if running
pkill -f "uvicorn app.web_server:app" || true
pkill -f "scripts/game_player.py" || true
pkill -f "scripts/game_scoreboard.py" || true
pkill -f "scripts/game_master.py" || true
pkill -f "scripts/rmq_consume_.*\\.py" || true
pkill -f "scripts/rmq_publish_.*\\.py" || true

echo "[teardown] Stopping Docker services..."
if docker compose version >/dev/null 2>&1; then
  docker compose down -v --remove-orphans || true
else
  docker-compose down -v --remove-orphans || true
fi

echo "[teardown] Removing leftover container/network if any..."
docker rm -f demo-event-bus-rabbitmq-1 2>/dev/null || true
docker network rm demo-event-bus_default 2>/dev/null || true

echo "[teardown] Freeing ports 5672 and 15672 if still in use..."
for port in 5672 15672; do
  pids="$(lsof -ti TCP:$port 2>/dev/null || true)"
  if [[ -n "${pids}" ]]; then
    echo "[teardown] Killing processes on port $port: $pids"
    kill -9 $pids || true
  fi
done

echo "[teardown] Done. To start again:"
echo "  docker compose up -d"
echo "  PYTHONPATH=. uvicorn app.web_server:app --reload --port 8000"