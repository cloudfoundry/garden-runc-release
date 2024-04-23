#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR="${THIS_FILE_DIR}/../.."
LOCATION="$WORKSPACE_DIR/artifacts"
CI="${THIS_FILE_DIR}/../../wg-app-platform-runtime-ci"
. "$CI/shared/helpers/git-helpers.bash"
REPO_NAME=$(git_get_remote_name)
REPO_PATH="${THIS_FILE_DIR}/../"
unset THIS_FILE_DIR

IMAGE="cloudfoundry/tas-runtime-build"
CONTAINER_NAME="$REPO_NAME-docker-container"

if [[ -z "${*}" ]]; then
  ARGS="-it"
else
  ARGS="${*}"
fi

pushd $REPO_PATH > /dev/null
bosh sync-blobs
popd > /dev/null

pushd "$CI/garden-runc-release/dockerfiles"
LOCATION=${LOCATION} make
popd

if [[ -f "${HOME}/workspace/devenv/functions/gcp-secret-manager-helpers.bash" ]]; then
  . "${HOME}/workspace/devenv/functions/gcp-secret-manager-helpers.bash"
  export DOCKER_REGISTRY_USERNAME="$(gimme-secret-value-only dockerhub-tasruntime-username)"
  export DOCKER_REGISTRY_PASSWORD="$(gimme-secret-value-only dockerhub-tasruntime-password)"
fi
if [[ "${DOCKER_REGISTRY_USERNAME:-undefined}" == "undefined" || "${DOCKER_REGISTRY_PASSWORD:-undefined}" == "undefined" ]]; then
  cat << EOF
  Run this script with DOCKER_REGISTRY_USERNAME, DOCKER_REGISTRY_PASSWORD env variables
EOF
exit 1
fi

docker pull "${IMAGE}"
docker rm -f $CONTAINER_NAME
docker run -it \
  --env "REPO_NAME=$REPO_NAME" \
  --env "REPO_PATH=/repo" \
  --env "GARDEN_TEST_ROOTFS=/artifacts/garden-rootfs.tar" \
  --env "GARDEN_FUSE_TEST_ROOTFS=/artifacts/garden-fuse.tar" \
  --env "DOCKER_REGISTRY_USERNAME=$DOCKER_REGISTRY_USERNAME" \
  --env "DOCKER_REGISTRY_PASSWORD=$DOCKER_REGISTRY_PASSWORD" \
  --rm \
  --name "$CONTAINER_NAME" \
  -v "${REPO_PATH}:/repo" \
  -v "${CI}:/ci" \
  -v "${LOCATION}:/artifacts" \
  --privileged \
  --cap-add ALL \
  ${ARGS} \
  "${IMAGE}" \
  /bin/bash
  
