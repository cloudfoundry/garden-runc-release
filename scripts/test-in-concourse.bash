#!/bin/bash

set -eu
set -o pipefail

THIS_FILE_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" >/dev/null 2>&1 && pwd )"
CI="$(realpath "${THIS_FILE_DIR}/../../wg-app-platform-runtime-ci")"
. "$CI/shared/helpers/git-helpers.bash"
REPO_NAME=$(git_get_remote_name)

"$THIS_FILE_DIR/fly/build-binaries.bash"

package="${1:-no_package}"
if [[ "${package}" != "no_package" ]]; then
  "$THIS_FILE_DIR/fly/test.bash" "${package}"
else
  internal_repos=$(yq -r '.internal_repos|.[].name' "$CI/$REPO_NAME/index.yml")
  for component in $internal_repos; do
    "$THIS_FILE_DIR/fly/test.bash" "${component}"
  done
fi

