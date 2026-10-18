#!/usr/bin/env bash
#
# Seed AgentPlane with a realistic demo organization.
#
# Targets the docker-compose stack using its published host ports. Override any
# value via the environment before running, e.g.:
#
#   DATABASE_URL=postgres://... ./seed.sh --reset
#
# Usage:
#   ./seed.sh                 # add demo data (idempotent)
#   ./seed.sh --reset         # wipe demo data first, then reseed (recommended)
#   ./seed.sh --reset --days 14 --traces 20000
#
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"

# Postgres is published on 5433 by docker-compose; ClickHouse native on 9000.
export DATABASE_URL="${DATABASE_URL:-postgres://agentplane:agentplane@localhost:5433/agentplane?sslmode=disable}"
export CLICKHOUSE_HOST="${CLICKHOUSE_HOST:-localhost}"
export CLICKHOUSE_PORT="${CLICKHOUSE_PORT:-9000}"
export CLICKHOUSE_HTTP_PORT="${CLICKHOUSE_HTTP_PORT:-8123}"
export CLICKHOUSE_USER="${CLICKHOUSE_USER:-default}"
export CLICKHOUSE_PASSWORD="${CLICKHOUSE_PASSWORD:-agentplane}"

# Must match the gateway's pepper (vault-init writes this same dev value) so
# minted keys authenticate. The gateway reads it from Vault; the seed reads it
# from the environment.
export API_KEY_PEPPER="${API_KEY_PEPPER:-dev-pepper-change-me}"
export AGENTPLANE_ADMIN_TOKEN="${AGENTPLANE_ADMIN_TOKEN:-dev-admin-token-change-me}"
export SECRET_STORE="${SECRET_STORE:-env}"

# Where the gateway should send chat traffic for the seeded providers. The mock
# upstream keeps the demo working end to end.
export SEED_PROVIDER_BASE_URL="${SEED_PROVIDER_BASE_URL:-http://mock:3002/v1}"
export GATEWAY_URL="${GATEWAY_URL:-http://localhost:3001}"

exec go run ./cmd/seed "$@"
