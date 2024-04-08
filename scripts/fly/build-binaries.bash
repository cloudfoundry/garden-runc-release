#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
WORKSPACE_DIR="${THIS_FILE_DIR}/../../.."
BUILT_BINARIES="$WORKSPACE_DIR/built-binaries/garden-runc-release"
CI="${WORKSPACE_DIR}/wg-app-platform-runtime-ci"
. "$CI/shared/helpers/git-helpers.bash"
REPO_NAME=$(git_get_remote_name)
REPO_PATH="${THIS_FILE_DIR}/../../"
unset THIS_FILE_DIR

if [[ "${CLEAN_CACHE:-no}" == "yes" ]]; then
  rm -rf "${BUILT_BINARIES}"
fi

if [[ ! -d "${BUILT_BINARIES}" ]]; then
  DEFAULT_PARAMS="ci/$REPO_NAME/default-params/build-binaries/linux.yml" "$CI/bin/fly-exec.bash" build-binaries -i repo="${REPO_PATH}" -o built-binaries="${BUILT_BINARIES}"
fi
