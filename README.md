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
- [Gosub](https://github.com/vito/gosub) - Gosub is a submodule based dependency manager for Go
- [Fly CLI](https://github.com/concourse/fly) - Concourse CLI
- [Virtualbox](https://www.virtualbox.org/) - Virtualization box
- [Vagrant](https://www.vagrantup.com/) - Portable dev environment

Garden-runC uses git submodules to maintain its dependencies and components.
Some of Garden-runC's important components currently are:

* [Garden](https://github.com/cloudfoundry/garden) found under
   `src/code.cloudfoundry.org/garden` is the API server and client.
* [Guardian](https://github.com/cloudfoundry/guardian) found under
   `src/code.cloudfoundry.org/guardian` is the Garden backend.
* [GrootFS](https://github.com/cloudfoundry/grootfs) found under
   `src/code.cloudfoundry.org/grootfs` downloads and manages
   root filesystems.
* [Garden Shed](https://github.com/cloudfoundry/garden-shed) found under
   `src/code.cloudfoundry.org/garden-shed` is the legacy rootfs management piece,
   superseded by GrootFS.
* [GATS](https://github.com/cloudfoundry/garden-integration-tests)
   found under `src/code.cloudfoundry.org/garden-integration-tests`
   are the cross-backend integration tests of Garden.

Set your `$GOPATH` to the checked out directory, or use Direnv to do this, as
below:

```bash
direnv allow
```

### Running the tests

[Concourse CI](https://concourse.ci/) is used for running Garden-runC tests
in a VM. It provides the [Fly CLI](https://github.com/concourse/fly) for
Linux and MacOSX. Instructions for deploying a single VM Concourse using BOSH
can be found in the [concourse-deployment repo](https://github.com/concourse/concourse-deployment)

Once running, navigate to [https://192.168.100.4:8080](https://192.168.100.4:8080) in a web browser
and download the [Fly CLI](https://concourse.ci/fly-cli.html) using the links found in
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
./scripts/remote-fly ci/unit-tests/guardian.yml

# Running Garden tests
./scripts/remote-fly ci/unit-tests/garden.yml

# Running Garden integration tests
./scripts/remote-fly ci/integration-tests/gdn-linux.yml
```

#### Running the tests locally

It is possible to run the integration tests locally on a Linux based OS like Ubuntu, but we don't recommend it
due to the dependencies required, and the need for parts of the testing suite to run as a privileged user. 
If you'd like to run them locally, you will need at least:
* A recent version of Go (1.8+)
* Kernel version 4.4+
* Running as a privileged user
* [AUFS](https://aufs.sourceforge.net)
* [Overlayfs](https://www.kernel.org/doc/Documentation/filesystems/overlayfs.txt)
* [xfs](http://xfs.org)

The tests can be executed without Concourse CLI by running `ginkgo -r`
command for any of the components:

```bash
# Running Garden unit tests
cd src/code.cloudfoundry.org/garden
ginkgo -r

# Running Guardian unit tests
cd src/code.cloudfoundry.org/guardian
ginkgo -r
```

It should be possible to run the unit tests on any system that satisfies golang build constraints.

#### Committing code

Write code in a submodule:

```bash
cd src/code.cloudfoundry.org/guardian # for example
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

#### Execute command in container using runc

- Create a container with handle &lt;handle&gt;
- Log in to the host OS with root privileges
- Go to container depot directory
```
cd /var/vcap/data/garden/depot/<handle>
```
- Execute process in container
```
/var/vcap/packages/runc/bin/runc exec <handle> <command>
```

### Troubleshooting

The garden-ordnance-survey tool can be used to gather information useful for
debugging issues on garden-runc-release deployments. Run this command on the
deployment VM as root:

`curl bit.ly/garden-ordnance-survey -sSfL | bash`

### License

Apache License 2.0
