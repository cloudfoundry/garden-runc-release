#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR="${THIS_FILE_DIR}/../../.."
LOCATION="$WORKSPACE_DIR/artifacts"
BUILT_BINARIES="$WORKSPACE_DIR/built-binaries/garden-runc-release"
CI="${WORKSPACE_DIR}/wg-app-platform-runtime-ci"
. "$CI/shared/helpers/git-helpers.bash"
REPO_NAME=$(git_get_remote_name)
REPO_PATH="${THIS_FILE_DIR}/../../"
unset THIS_FILE_DIR

pushd $REPO_PATH > /dev/null
bosh sync-blobs
popd > /dev/null

pushd "$CI/garden-runc-release/dockerfiles"
LOCATION=${LOCATION} make
popd

echo "Testing ${1}"

ROOTFS_ENVS='GARDEN_TEST_ROOTFS=$PWD/input-01/garden-rootfs.tar
GARDEN_FUSE_TEST_ROOTFS=$PWD/input-01/garden-fuse.tar'

if [[ -f "${HOME}/workspace/devenv/functions/gcp-secret-manager-helpers.bash" ]]; then
  . "${HOME}/workspace/devenv/functions/gcp-secret-manager-helpers.bash"
  export DOCKER_REGISTRY_USERNAME="$(gimme-secret-value-only dockerhub-tasruntime | yq -r .user)"
  export DOCKER_REGISTRY_PASSWORD="$(gimme-secret-value-only dockerhub-tasruntime | yq -r .password)"
fi

if [[ "${DOCKER_REGISTRY_USERNAME:-undefined}" == "undefined" || "${DOCKER_REGISTRY_PASSWORD:-undefined}" == "undefined" ]]; then
  cat << EOF
  Run this script with DOCKER_REGISTRY_USERNAME, DOCKER_REGISTRY_PASSWORD env variables
EOF
exit 1
fi

ENVS="$ROOTFS_ENVS
DOCKER_REGISTRY_USERNAME="$DOCKER_REGISTRY_USERNAME"
DOCKER_REGISTRY_PASSWORD="$DOCKER_REGISTRY_PASSWORD"
${ENVS:-}" \
DIR="src/${1}" \
"$CI/bin/fly-exec.bash" run-bin-test -i repo="${REPO_PATH}" -i built-binaries="${BUILT_BINARIES}" -i input-01="${LOCATION}" -p
