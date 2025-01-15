#!/bin/bash

set -eu
set -o pipefail

if [ -f /sys/fs/cgroup/cgroup.controllers ]; then
    echo "Setting up controllers for cgroup v2"
    # move the processes from the root group to the /init group,
    # otherwise writing subtree_control fails with EBUSY
    # An error during moving non-existent process (i.e., "cat") is ignored.
    mkdir -p /sys/fs/cgroup/init
    xargs -rn1 < /sys/fs/cgroup/cgroup.procs > /sys/fs/cgroup/init/cgroup.procs || :
    # enable controllers
    sed -e 's/ / +/g' -e 's/^/+/' < /sys/fs/cgroup/cgroup.controllers > /sys/fs/cgroup/cgroup.subtree_control
fi

. "/ci/shared/helpers/git-helpers.bash"

function test() {
  local package="${1:?Provide a package}"
  local sub_package="${2:-}"

  export DIR=${package}
  . <(/ci/shared/helpers/extract-default-params-for-task.bash /ci/shared/tasks/run-bin-test/linux.yml)

  export GOFLAGS="-buildvcs=false"
  /ci/shared/tasks/run-bin-test/task.bash "${sub_package}"
}

pushd /repo > /dev/null
git_configure_safe_directory
REPO_NAME=$(git_get_remote_name)
export DEFAULT_PARAMS="/ci/$REPO_NAME/default-params/run-bin-test/linux.yml"
popd > /dev/null

pushd / > /dev/null
if [[ -n "${1:-}" ]]; then
  test "src/${1}" "${2:-}"
else
  internal_repos=$(yq -r '.internal_repos[]  | select(.acceptance != true) | .name' "/ci/$REPO_NAME/index.yml")
  for component in $internal_repos; do
    test "src/${component}"
  done
fi
popd > /dev/null
