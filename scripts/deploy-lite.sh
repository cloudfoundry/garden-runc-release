#!/usr/bin/env bash
set -euo pipefail

BOSH_ENVIRONMENT=${BOSH_ENVIRONMENT:-lite}

manifests=$(cd "$(dirname "$0")/../manifests" && pwd)
bosh -n -e "$BOSH_ENVIRONMENT" -d garden-runc deploy --recreate --no-redact \
  -l "$manifests/bosh-lite.vars.yml" \
  "$@" \
  "$manifests/garden-runc.yml"
