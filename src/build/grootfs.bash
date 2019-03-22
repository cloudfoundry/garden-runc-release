function build_grootfs() {
  local garden_runc_release_dir="$1"
  local target="$2"

  (
    cd "$garden_runc_release_dir/src/grootfs" || exit
    make all

    chmod 04755 build/tardis

    mv build/{grootfs,tardis} "$target"
  )
}
