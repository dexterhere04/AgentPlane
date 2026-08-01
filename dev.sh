#!/usr/bin/env bash
set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

cleanup() {
  echo ""
  echo "Shutting down..."
  kill "$MOCK_PID" 2>/dev/null
  kill "$AGENT_PID" 2>/dev/null
  wait "$MOCK_PID" 2>/dev/null
  wait "$AGENT_PID" 2>/dev/null
  echo "Done."
}

trap cleanup EXIT INT TERM

echo "=== AgentPlane Dev Mode ==="
echo ""

echo "[1/2] Starting Mock API on :3002..."
cd "$SCRIPT_DIR/testing"
go run ./cmd/mockapi/ &
MOCK_PID=$!
sleep 1

echo "[2/2] Starting AgentPlane on :3001..."
cd "$SCRIPT_DIR"
OPENAI_API_KEY=mock-key OPENAI_BASE_URL=http://localhost:3002/v1 go run ./cmd/server/ &
AGENT_PID=$!
sleep 1

echo ""
echo "----------------------------------------"
echo "  Dashboard → http://localhost:3001/dashboard"
echo "  Gateway   → http://localhost:3001/chat"
echo "  Events    → http://localhost:3001/events"
echo "  Mock API  → http://localhost:3002/health"
echo "----------------------------------------"
echo ""
echo "Press Ctrl+C to stop."

wait
