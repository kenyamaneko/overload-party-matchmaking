#!/usr/bin/env bash
# generate_types.sh — data/{openapi,asyncapi}.yaml から packages/api-matchmaking の Go 型を再生成する。
#
# REST 部分は oapi-codegen、Pub/Sub 部分は overload-party-asyncapi-codegen-tools (common 由来) を使う。
set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"

cd "$REPO_ROOT/packages/api-matchmaking"
oapi-codegen -config openapi-codegen.yaml ../../data/openapi.yaml

cd "$REPO_ROOT"
asyncapi-codegen \
  --input data/asyncapi.yaml \
  --output packages/api-matchmaking/asyncapi_gen.go \
  --package apimatchmaking

gofmt -w packages/api-matchmaking/openapi_gen.go
