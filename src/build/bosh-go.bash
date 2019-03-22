# These need to be defined since bosh compilation VMs don't set $HOME
export GOCACHE=/tmp/gocache
export GOPATH=/tmp/gopath

export GOROOT; GOROOT="$( readlink -nf /var/vcap/packages/golang )"
export PATH="${GOROOT}/bin:${PATH}"
