# Guardian Release

A [BOSH](http://docs.cloudfoundry.org/bosh/) release for deploying
[Guardian](https://github.com/cloudfoundry-incubator/guardian).

Guardian is a simple single-host [OCI](https://opencontainers.org/) container
manager. It implements the [Garden](https://github.com/cloudfoundry-incubator/garden/)
API which is used in [Cloud Foundry](https://www.cloudfoundry.org/).


## Getting started

Clone it:

```bash
git clone https://github.com/cloudfoundry-incubator/guardian-release
cd guardian-release
git submodule update --init --recursive
```

### Running

Guardian runs on linux, therefore you need to have a linux machine available.
The easiest way to run Guardian is to deploy it with [BOSH](https://bosh.io) to
a local Linux VM managed by [Vagrant](https://www.vagrantup.com).

In order to run Guardian in a VM you need to have the following installed:

- [Virtualbox](https://www.virtualbox.org/) - Virtualization hypervisor
- [Vagrant](https://www.vagrantup.com/) - Development environment management
- [BOSH CLI](https://bosh.io/docs/bosh-cli.html) - A CLI to interact with [BOSH](ttps://bosh.io)

Create a `bosh-lite` VM and deploy Guardian:

```bash
git clone https://github.com/cloudfoundry/bosh-lite
cd bosh-lite
vagrant up --provider=virtualbox

bosh target 192.168.50.4 lite
bosh upload stemcell https://bosh.io/d/stemcells/bosh-warden-boshlite-ubuntu-trusty-go_agent

cd ../guardian-release

bosh --non-interactive create release --force
bosh upload release
bosh deployment manifests/bosh-lite.yml
bosh --non-interactive deploy
```

You can retrieve the address of the Guardian server by running `bosh vms`.  It
will be `10.244.16.6` by default. The server port defaults to `7777`.

### Usage

The easiest way to start creating containers in Guardian is
[`gaol`](https://github.com/contraband/gaol) command line client.

For example:

```bash
go get github.com/contraband/gaol

# create a container
gaol -t 10.244.16.6:7777 create -n my-container

# run a command inside the container
gaol -t 10.244.16.6:7777 run my-container --attach --command "uname -a"

# shell into the container
gaol -t 10.244.16.6:7777 shell my-container

# destroy the container
gaol -t 10.244.16.6:7777 destroy my-container
```
For more advanced use cases, you'll need to use the [Garden
client](https://godoc.org/github.com/cloudfoundry-incubator/garden#Client)
package for Golang.

## Contributing

In order to help us extend Guardian, we recommend opening a Github issue
to describe the proposed features or changes. We also welcome pull requests.

Guardian is a Linux project and it currently supports only Ubuntu 14.04 LTS.

You can use other distributions or MacOSX for development since a good chunk
of the unit tests work across alternative platforms and you can run platform
specific tests in a VM. [Concourse CI](https://concourse.ci/) system is used
for running Guardian tests in a VM. Concourse provides its [Fly
CLI](https://github.com/concourse/fly) for Linux and MacOSX as well.

In order to contribute to the project you should have the following installed:

- [Git](https://git-scm.com/) - Distributed version control system
- [Go 1.4](https://golang.org/doc/install#install) - The Go programming
   language
- [Direnv](https://github.com/direnv/direnv) - Environment management
- [Gosub](https://github.com/vito/gosub) - Gosub is a submodule based dependency manager for Go
- [Fly CLI](https://github.com/concourse/fly) - Concourse CLI
- [Virtualbox](https://www.virtualbox.org/) - Virtualization box
- [Vagrant](https://www.vagrantup.com/) - Portable dev environment
- [Aufs](http://aufs.sourceforge.net/) - Another Union File System for Linux

### Getting the source code

Guardian uses git submodules to maintain its dependencies and components.
Guardian's components currently are:

* [Garden](https://github.com/cloudfoundry-incubator/garden) found under
   `src/github.com/cloudfoundry-incubator/garden` is the API server and client.
* [Guardian](https://github.com/cloudfoundry-incubator/guardian) found under
   `src/github.com/cloudfoundry-incubator/guardian` is the Garden backend.
* [GITS](https://github.com/cloudfoundry-incubator/garden-integration-tests)
   found under `src/github.com/cloudfoundry-incubator/garden-integration-tests`
   are the cross-backend integration tests of Garden.


Start by checking out/updating the submodules:

```bash
git submodule update --init --recursive
```

Set your `$GOPATH` to the checked out directory, or use Direnv to do this, as
below:

```bash
direnv allow
```

Write code in a submodule:

```bash
cd src/github.com/cloudfoundry-incubator/guardian # for example
git checkout master
git pull
# test, code, test..
git commit
git push
```

Commit the changes and create a bump commit:

```bash
# from the guardian-release directory
./scripts/bump
```

### Running the tests

[Concourse CI](https://concourse.ci/) system is used for running Guardian tests
in a VM. It provides its [Fly CLI](https://github.com/concourse/fly) for
Linux and MacOSX. Concourse has a pre-built Vagrant box for VirtualBox. You
can create a new instance by executing the following commands:

```bash
mkdir concourse-lite
cd concourse-lite
vagrant init concourse/lite
vagrant up
```

Open [http://192.168.100.4:8080](http://192.168.100.4:8080) in a web browser
and download the [Fly CLI](http://concourse.ci/fly-cli.html) from the
bottom-right corner. Place the `fly` binary in a directory referenced from the
`$PATH` environment variable.

The tests are using [Ginkgo](https://onsi.github.io/ginkgo/) BDD testing
framework, which can be installed with the following command:

```bash
go install github.com/onsi/ginkgo
```

You can run all the tests by executing `./scripts/test`.

#### Unit tests

If you are using Linux based operation system like Ubuntu, you should install
[Aufs](http://aufs.sourceforge.net/) before run any tests:

```bash
sudo apt-get install linux-image-extra-$(uname -r)
sudo modprobe aufs
```

The unit tests can be executed without Concourse CLI by running `ginkgo -r`
command for any of the components:

```bash
# Running Garden unit tests
cd src/github.com/cloudfoundry-incubator/garden
ginkgo -r

# Running Guardian unit tests
cd src/github.com/cloudfoundry-incubator/guardian
ginkgo -r
```

#### Integration tests

The integration tests can be executed in Concourse CI by using Fly CLI and
executing `./scripts/remote-fly`:

```bash
# Running Guardian tests
./scripts/remote-fly ci/nested-guardian-tests.yml

# Running Garden integration tests in Concourse CI
./scripts/remote-fly ci/nested-tests.yml
```

### License

Apache License 2.0
