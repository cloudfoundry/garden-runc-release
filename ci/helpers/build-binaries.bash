function build_runc() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/vendor/github.com/opencontainers/runc" || exit

    make BUILDTAGS='seccomp apparmor' static
    mv runc "${target}"
  )
}

function build_grootfs() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/grootfs" || exit

    make clean
    if [  "${WITH_MUSL:-no}" == "no" ]; then
      make
    else
      CC="${WITH_MUSL}" STATIC_BINARY=true make
    fi
    make prefix="${target}" install
  )
}

function build_nstar() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/rundmc/nstar" || exit
    make clean
    if [  "${WITH_MUSL:-no}" == "no" ]; then
      make
    else
      CC="${WITH_MUSL}" make
    fi
    mv nstar "${target}"
  )
}

function build_init() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/cmd/init" || exit

    if [  "${WITH_MUSL:-no}" == "no" ]; then
      gcc -static -o init init.c ignore_sigchild.c
    else
      CC="${WITH_MUSL}" gcc -static -o init init.c ignore_sigchild.c
    fi

    mv init "${target}/init"
  )
}

function build_dadoo() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian" || exit

    go build -o "${target}/dadoo" ./cmd/dadoo

  )
}

function build_containerd() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/vendor/github.com/containerd/containerd" || exit

    BUILDTAGS=no_btrfs make ./bin/containerd
    BUILDTAGS=no_btrfs make ./bin/containerd-shim
    BUILDTAGS=no_btrfs make ./bin/containerd-shim-runc-v1
    BUILDTAGS=no_btrfs make ./bin/containerd-shim-runc-v2
    BUILDTAGS=no_btrfs make ./bin/ctr
    mv bin/* "${target}"
  )
}

function build_idmapper_binaries() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/idmapper" || exit

    go build -o "${target}/newuidmap" ./cmd/newuidmap
    go build -o "${target}/newgidmap" ./cmd/newgidmap
    go build -o "${target}/maximus" ./cmd/maximus
  )
}

function build_musl() {
  local release="$1"
  local target="$2"
  local musl_tmp_dir="$(mktemp -d --suffix '-musl-tmp-dir')"
  local musl_tarball="$(ls $release/blobs/musl/musl-*.tar.gz)"

  tar xzf "$musl_tarball" -C "$musl_tmp_dir" --strip-components=1
  (
     cd "$musl_tmp_dir" || exit
     ./configure --prefix="$target"
     make install
  )

  ln -s /usr/include/linux "$target/include/"
  ln -s /usr/include/asm-generic "$target/include/"
  ln -s /usr/include/asm-generic "$target/include/asm"

  rm -rf "$musl_tmp_dir"
}
