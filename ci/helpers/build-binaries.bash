function build_runc() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/gopath/src/github.com/opencontainers/runc" || exit

    make BUILDTAGS='seccomp apparmor' static
    mv runc "${target}"
  )
}

function build_grootfs() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/grootfs" || exit

    make
    make prefix="${target}" install
  )
}

function build_nstar() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/rundmc/nstar" || exit
    make
    mv nstar "${target}"
  )
}

function build_init() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian/cmd/init" || exit

    gcc -static -o init init.c ignore_sigchild.c
    mv init "${target}/init"
  )
}

function build_dadoo() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/guardian" || exit

    go build -mod vendor -o "${target}/dadoo" ./cmd/dadoo

    cd ./cmd/init || exit

    gcc -static -o init init.c ignore_sigchild.c
    mv init "${target}/init"
  )
}

function build_containerd() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/gopath/src/github.com/containerd/containerd" || exit

    BUILDTAGS=no_btrfs make
    mv bin/* "${target}"
  )
}

function build_idmapper_binaries() {
  local release="$1"
  local target="$2"

  (
    cd "${release}/src/idmapper" || exit

    go build -mod vendor -o "${target}/newuidmap" ./cmd/newuidmap
    go build -mod vendor -o "${target}/newgidmap" ./cmd/newgidmap
    go build -mod vendor -o "${target}/maximus" ./cmd/maximus
  )
}
