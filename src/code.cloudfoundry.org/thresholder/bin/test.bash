#!/bin/bash

set -eu
set -o pipefail

source "$CI_DIR/shared/helpers/filesystem-helpers.bash"
filesystem_permit_device_control
filesystem_create_loop_devices 256

# shellcheck disable=SC2068
# Double-quoting array expansion here causes ginkgo to fail
go run github.com/onsi/ginkgo/v2/ginkgo ${@}
