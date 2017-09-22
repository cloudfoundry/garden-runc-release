#!/usr/bin/env bash
set -euo pipefail

(
cd "$(dirname "$0")/.."
bosh -n -e "${BOSH_ENVIRONMENT:-lite}" -d garden-runc deploy --no-redact \
  -l manifests/bosh-lite.vars.yml \
  -o manifests/local-garden-runc-release.ops.yml \
  "$@" \
  manifests/garden-runc.yml
)
