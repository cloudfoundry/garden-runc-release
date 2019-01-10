# shellcheck source=../patches/patch.bash
source "$(dirname "${BASH_SOURCE[0]}")"/../patches/patch.bash

function build_runc() {
  garden_runc_release_dir="$1"
  dest_dir="$2"

  pushd "$garden_runc_release_dir/src/github.com/opencontainers/runc"
    apply_patches "$garden_runc_release_dir/src/patches/runc"
    make BUILDTAGS='seccomp apparmor' static
    mv runc "$dest_dir"
  popd
}
