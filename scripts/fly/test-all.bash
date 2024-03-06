#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
CI="${THIS_FILE_DIR}/../../../wg-app-platform-runtime-ci"
. "$CI/shared/helpers/git-helpers.bash"
REPO_NAME=$(git_get_remote_name)

internal_repos=$(yq -r '.internal_repos|.[].name' "$CI/$REPO_NAME/index.yml")
for component in $internal_repos; do
  "$THIS_FILE_DIR/test.bash" "${component}"
done
