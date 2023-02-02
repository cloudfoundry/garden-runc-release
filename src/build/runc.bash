# shellcheck source=../patches/patch.bash

function build_runc() {
  local garden_runc_release_dir="$1"
  local target="$2"

  (
    cd "$garden_runc_release_dir/src/gopath/src/github.com/opencontainers/runc" || exit

    make BUILDTAGS='seccomp apparmor' static
    mv runc "$target"
  )
}
