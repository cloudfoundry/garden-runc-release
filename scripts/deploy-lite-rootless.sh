#!/usr/bin/env bash
set -euo pipefail

manifests=$(cd "$(dirname "$0")/../manifests" && pwd)
"$(dirname "$0")"/deploy-lite.sh \
  -o "$manifests/bpm.ops.yml" \
  -o "$manifests/noop-networking.ops.yml" \
  -o "$manifests/rootless.ops.yml" \
  --vars-store creds.yml "$@"
