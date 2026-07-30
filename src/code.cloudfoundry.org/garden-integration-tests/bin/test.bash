#!/bin/bash

set -eu
set -o pipefail

source "$CI_DIR/shared/helpers/filesystem-helpers.bash"
filesystem_permit_device_control
filesystem_create_loop_devices 256

garden_rootfs_ext="${GARDEN_TEST_ROOTFS##*.}"
if [[ $garden_rootfs_ext != "tar" ]]; then
   garden_rootfs_tar="$(dirname $GARDEN_TEST_ROOTFS)/garden-rootfs.tar"
   tar -cf "$garden_rootfs_tar" -C $GARDEN_TEST_ROOTFS .
   export GARDEN_TEST_ROOTFS=$garden_rootfs_tar
fi

configure_gdn

mkdir -p /var/lib/grootfs
# shellcheck disable=SC2068
# Double-quoting array expansion here causes ginkgo to fail
go run github.com/onsi/ginkgo/v2/ginkgo ${@}
