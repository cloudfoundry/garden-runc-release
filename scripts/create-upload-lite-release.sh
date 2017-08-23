#!/usr/bin/env bash
set -euo pipefail

(
cd "$(dirname "$0")/.."
bosh create-release --force
BOSH_ENVIRONMENT=${BOSH_ENVIRONMENT:-lite}
bosh -e "$BOSH_ENVIRONMENT" upload-release --rebase
)
