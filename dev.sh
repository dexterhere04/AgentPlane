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

fuser -k 3001/tcp 2>/dev/null || true
fuser -k 3002/tcp 2>/dev/null || true
sleep 1

echo "[1/3] Building mock API..."
cd "$SCRIPT_DIR/testing"
go build -o /tmp/mockapi ./cmd/mockapi/
echo "       Mock API built."

echo "[2/3] Building AgentPlane..."
cd "$SCRIPT_DIR"
go build -o /tmp/agentplane ./cmd/server/
echo "       AgentPlane built."

echo "[3/3] Starting services..."
/tmp/mockapi > /tmp/mock.log 2>&1 &
MOCK_PID=$!
sleep 1

# The gateway now requires PostgreSQL for users/API keys. Start it with:
#   docker compose up -d postgres
# (migrations are applied automatically on startup).
SECRET_STORE=env \
OPENAI_API_KEY=mock-key \
OPENAI_BASE_URL=http://localhost:3002/v1 \
DATABASE_URL=postgres://agentplane:agentplane@localhost:5432/agentplane?sslmode=disable \
API_KEY_PEPPER=dev-pepper-change-me \
AGENTPLANE_ADMIN_TOKEN=dev-admin-token-change-me \
GUARDRAIL_SECRETS=enforce \
GUARDRAIL_PII=enforce \
GUARDRAIL_PROMPT_INJECTION=enforce \
/tmp/agentplane > /tmp/agent.log 2>&1 &
AGENT_PID=$!
sleep 1

echo ""
echo "----------------------------------------"
echo "  Dashboard → http://localhost:3001/dashboard"
echo "  Gateway   → http://localhost:3001/chat"
echo "  Events    → http://localhost:3001/events"
echo "  Metrics   → http://localhost:3001/metrics"
echo "  Mock API  → http://localhost:3002/health"
echo ""
echo "  Guardrails: secrets + pii + prompt_injection (enforce mode)"
echo ""
echo "  Requires PostgreSQL (docker compose up -d postgres)"
echo "----------------------------------------"
echo ""
echo "Press Ctrl+C to stop."

wait
