# Garden-runC Release

A [BOSH](http://docs.cloudfoundry.org/bosh/) release for deploying
[Guardian](https://github.com/cloudfoundry/guardian).

Guardian is a simple single-host [OCI](https://opencontainers.org/) container
manager. It implements the [Garden](https://github.com/cloudfoundry/garden/)
API which is used in [Cloud Foundry](https://www.cloudfoundry.org/).

## Getting started

Clone it:

```bash
git clone https://github.com/cloudfoundry/garden-runc-release
cd garden-runc-release
git submodule update --init --recursive
```

### Running

The easiest way to run Garden-runC is to deploy it with [BOSH
Lite](https://bosh.io/docs/bosh-lite.html), a VirtualBox development
environment for [BOSH](https://bosh.io). Once you have  set up bosh-lite
(follow the instructions in the bosh-lite docs), just deploy like any bosh
release, e.g:

```bash
cd garden-runc-release # if you're not already there
./scripts/deploy-lite.sh
```

You can retrieve the address of the Garden-runC server by running `bosh vms`.
It will be `10.244.0.2` if using the provided deploy-lite script.
The server port defaults to `7777`.

### Usage

The easiest way to start creating containers is to use the
[`gaol`](https://github.com/contraband/gaol) command line client.

e.g. `gaol -t 10.244.0.2:7777 create -n my-container`

For more advanced use cases, you'll need to use the [Garden
client](https://godoc.org/code.cloudfoundry.org/garden#Client)
package for Golang.

### Operating garden-runc

[Operator's guide.](docs/opsguide.md)

### Security Features

The following doc provides an overview of security features on Garden vs Docker vs Kubernetes.

[Security overview.](docs/security-overview.md)

### Rootless containers

Garden has experimental support for running containers without requiring root
privileges. Take a look at the
[rootless-containers.md](docs/articles/rootless-containers.md) doc for further info.

If you would like to enable rootless containers please read [this
document](docs/enabling-rootless-containers.md).

## Contributing

In order to help us extend Garden-runC, we recommend opening a Github issue to
describe the proposed features or changes. We also welcome pull requests.

You can use other distributions or OS X for development since a good chunk of
the unit tests work across alternative platforms, and you can run platform
specific tests in a VM using [Concourse CI](https://concourse.ci/).

In order to contribute to the project you may want some of the following installed:

- [Git](https://git-scm.com/) - Distributed version control system
- [Go](https://golang.org/doc/install#install) - The Go programming
   language
- [Direnv](https://github.com/direnv/direnv) - Environment management
- [Fly CLI](https://github.com/concourse/fly) - Concourse CLI
- [Virtualbox](https://www.virtualbox.org/) - Virtualization box
- [Vagrant](https://www.vagrantup.com/) - Portable dev environment

Garden-runC uses git submodules to maintain its dependencies and components.
Some of Garden-runC's important components currently are:

* [Garden](https://github.com/cloudfoundry/garden) found under
   `src/garden` is the API server and client.
* [Guardian](https://github.com/cloudfoundry/guardian) found under
   `src/guardian` is the Garden backend.
* [GrootFS](https://github.com/cloudfoundry/grootfs) found under
   `src/grootfs` downloads and manages
   root filesystems.
* [GATS](https://github.com/cloudfoundry/garden-integration-tests)
   found under `src/garden-integration-tests`
   are the cross-backend integration tests of Garden.

Update:
* [Garden Shed](https://github.com/cloudfoundry/garden-shed), previously found under
   `src/code.cloudfoundry.org/garden-shed`, has now been removed. GrootFS is now the default container
   rootfs management tool with no option to revert to Shed from versions above 1.16.8.

Set your `$GOPATH` to `<garden-runc-release-dir>/src/gopath`, or use Direnv to do this, as
below:

```bash
direnv allow
```

### Running the tests

[Concourse CI](https://concourse-ci.org/) is used for running Garden-runC tests
in a VM. It provides the [Fly CLI](https://github.com/concourse/fly) for
Linux and MacOSX. Instructions for deploying a single VM Concourse using BOSH
can be found in the [concourse-deployment repo](https://github.com/concourse/concourse-deployment)

Once running, navigate to [https://192.168.100.4:8080](https://192.168.100.4:8080) in a web browser
and download the [Fly CLI](https://concourse-ci.org/download.html) using the links found in
the bottom-right corner. Place the `fly` binary somewhere on your `$PATH`.

The tests use the [Ginkgo](https://onsi.github.io/ginkgo/) BDD testing
framework.

Assuming you have configured a Concourse and installed Ginkgo, you can run all
the tests by executing `FLY_TARGET=<your concourse target> ./scripts/test` from the top level `garden-runc-release` directory.

Note: The concourse-lite VM may need to be provisioned with more RAM
If you start to see tests failing with 'out of disk' errors.

#### Integration tests

The integration tests can be executed in Concourse CI by using Fly CLI and
executing `./scripts/test`.
To run individual tests, use`./scripts/remote-fly`:

```bash
# Set your concourse target
export GARDEN_REMOTE_ATC_URL=<target>

# Running Guardian tests
./scripts/remote-fly ci/tasks/guardian.yml

# Running Garden tests
./scripts/remote-fly ci/tasks/garden.yml

# Running Garden Integration tests
./scripts/remote-fly ci/tasks/gdn-linux.yml

# Running Garden Integration Windows Regression tests (aka Gats98)
WINDOWS_TEST_ROOTFS=docker:///microsoft/nanoserver:1709 ./scripts/remote-fly ci/tasks/gdn-linux.yml
```

#### Running the tests locally

It is possible to run the integration tests locally on a Linux based OS like Ubuntu, but we don't recommend it
due to the dependencies required, and the need for parts of the testing suite to run as a privileged user.
If you'd like to run them locally, you will need at least:
* A recent version of Go (1.8+)
* Kernel version 4.4+
* Running as a privileged user
* [Overlayfs](https://www.kernel.org/doc/Documentation/filesystems/overlayfs.txt)
* [xfs](http://xfs.org)

The tests can be executed without Concourse CLI by running `ginkgo -r`
command for any of the components:

```bash
# Running Garden unit tests
cd src/garden
ginkgo -r

# Running Guardian unit tests
cd src/guardian
ginkgo -r
```

It should be possible to run the unit tests on any system that satisfies golang build constraints.

#### Committing code

Write code in a submodule:

```bash
cd src/guardian # for example
git checkout master
git pull
# test, code, test..
git commit
git push
```

Commit the changes, run the tests, and create a bump commit:

```bash
# from the garden-runc directory
./scripts/test-and-bump # or just ./scripts/bump if you've already run the tests
```

### Troubleshooting

### Don't panic

#### >= v1.17.2:
If you have a problem with garden-runc, don't panic! There is a tool that you can
use to gather information useful for debugging issues on garden-runc-release
deployments. Run this command on the deployment VM as root:

`/var/vcap/packages/dontpanic/bin/dontpanic`

_N.B. From v1.18.3, if your BOSH environment has BPM enabled for Garden, `dontpanic` should still be run from
the host, not from within the BPM container._

#### <= v1.17.1:
If running an earlier version of Garden, you can download the latest release of `dontpanic` from [its repo here](https://github.com/cloudfoundry/dontpanic/releases) and run it as root
from the VM running the Garden job.

eg: `wget https://github.com/cloudfoundry/dontpanic/releases/download/v1.0/dontpanic && chmod +x ./dontpanic && ./dontpanic`.

_N.B. If your BOSH environment has BPM enabled for Garden, `dontpanic` should still be run from
the host, not from within the BPM container._

### License

Apache License 2.0
