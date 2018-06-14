#!/usr/bin/env bash
set -euo pipefail

DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

(
  cd "$( dirname "$DIR" )"
  ./scripts/deploy-lite.sh \
    -o manifests/cf-networking.ops.yml \
    --vars-store creds.yml
)
