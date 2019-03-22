# This needs to be defined since bosh compilation VMs don't set $HOME.
export GOCACHE=/tmp/gocache

# Since this is only to be sourced in bosh packaging scripts, we can assume
# that ./gopath is a GOPATH.
export GOPATH; GOPATH="$( readlink -nf ./gopath )"

export GOROOT; GOROOT="$( readlink -nf /var/vcap/packages/golang )"
export PATH="${GOROOT}/bin:${PATH}"
