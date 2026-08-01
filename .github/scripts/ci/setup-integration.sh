#!/usr/bin/env bash
set -euo pipefail

REDIS_PORT=6379
HEALTH_CHECK_TIMEOUT_SEC=30

docker run --detach --name matchmaking-ci-redis --publish "${REDIS_PORT}:6379" redis:7-alpine

is_ready=false
for _ in $(seq 1 "${HEALTH_CHECK_TIMEOUT_SEC}"); do
  if docker exec matchmaking-ci-redis redis-cli ping >/dev/null 2>&1; then
    is_ready=true
    break
  fi
  sleep 1
done
if [ "$is_ready" = false ]; then
  echo "Redis failed to start"
  docker logs matchmaking-ci-redis
  exit 1
fi
