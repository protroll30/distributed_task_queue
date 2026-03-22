#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
docker compose exec -T cockroach cockroach sql --insecure --host=localhost < migrations/001_initial.sql
echo "migration applied"
