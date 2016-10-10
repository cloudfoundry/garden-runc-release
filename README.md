# Garden-runC Release

A [BOSH](http://docs.cloudfoundry.org/bosh/) release for deploying
[Guardian](https://github.com/cloudfoundry/guardian).

Guardian is a simple single-host [OCI](https://opencontainers.org/) container
manager. It implements the [Garden](https://github.com/cloudfoundry/garden/) API
which is used in [Cloud Foundry](https://www.cloudfoundry.org/).

## Getting started

Clone it:

```bash
git clone https://github.com/cloudfoundry/garden-runc-release
cd garden-runc-release
git submodule update --init --recursive
```

### Running

The easiest way to run Garden-runC is to deploy it with [BOSH
Lite](https://github.com/cloudfoundry/bosh-lite), a local development
environment for [BOSH](https://bosh.io). Once you have  set up bosh-lite (follow the instructions in the bosh-lite repo), just deploy like any bosh release, e.g:

~~~~
cd garden-runc-release # if you're not already there
bosh deployment manifests/bosh-lite.yml
bosh create release && bosh upload release && bosh deploy
~~~~

You can retrieve the address of the Garden-runC server by running `bosh vms`.
It will be `10.244.16.6` if using the provided bosh-lite manifest in [manifests/bosh-lite.yml](https://github.com/cloudfoundry/garden-runc-release/blob/master/manifests/bosh-lite.yml). The server
port defaults to `7777`.

### Usage

The easiest way to start creating containers is to use the
[`gaol`](https://github.com/contraband/gaol) command line client.

e.g. `gaol -t 10.244.16.6:7777 create -n my-container`

For more advanced use cases, you'll need to use the [Garden
client](https://godoc.org/code.cloudfoundry.org/garden#Client)
package for Golang.

## Contributing

In order to help us extend Garden-runC, we recommend opening a Github issue to
describe the proposed features or changes. We also welcome pull requests.

You can use other distributions or OS X for development since a good chunk of
the unit tests work across alternative platforms, and you can run platform
specific tests in a VM using [Concourse CI](https://concourse.ci/).

In order to contribute to the project you should have the following installed:

- [Git](https://git-scm.com/) - Distributed version control system
- [Go 1.6](https://golang.org/doc/install#install) - The Go programming
   language
- [Direnv](https://github.com/direnv/direnv) - Environment management
- [Gosub](https://github.com/vito/gosub) - Gosub is a submodule based dependency manager for Go
- [Fly CLI](https://github.com/concourse/fly) - Concourse CLI
- [Virtualbox](https://www.virtualbox.org/) - Virtualization box
- [Vagrant](https://www.vagrantup.com/) - Portable dev environment

Garden-runC uses git submodules to maintain its dependencies and components.
Garden-runC's components currently are:

* [Garden](https://github.com/cloudfoundry/garden) found under
   `src/code.cloudfoundry.org/garden` is the API server and client.
* [Guardian](https://github.com/cloudfoundry/guardian) found under
   `src/code.cloudfoundry.org/guardian` is the Garden backend.
* [Garden Shed](https://github.com/cloudfoundry/garden-shed) found under
   `src/code.cloudfoundry.org/garden-shed` downloads and manages
   root filesystems.
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
bottom-right corner. Place the `fly` binary somewhere on your `$PATH`.

The tests use the [Ginkgo](https://onsi.github.io/ginkgo/) BDD testing
framework.

Assuming you have configured a Concourse and installed Ginkgo, you can run all
the tests by executing `./scripts/test`.

#### Unit tests

If you are using Linux based operation system like Ubuntu, you should install
[Aufs](http://aufs.sourceforge.net/) if you want to run tests on your local
machine.

```bash
sudo apt-get install linux-image-extra-$(uname -r)
sudo modprobe aufs
```

The unit tests can be executed without Concourse CLI by running `ginkgo -r`
command for any of the components:

```bash
# Running Garden unit tests
cd src/code.cloudfoundry.org/garden
ginkgo -r

# Running Guardian unit tests
cd src/code.cloudfoundry.org/guardian
ginkgo -r
```

#### Integration tests

The integration tests can be executed in Concourse CI by using Fly CLI and
executing `./scripts/remote-fly`:

```bash
# Running Guardian tests
./scripts/remote-fly ci/guardian.yml

# Running Garden Shed tests
./scripts/remote-fly ci/nested-shed-tests.yml

# Running Garden integration tests in Concourse CI
./scripts/remote-fly ci/gats.yml
```

#### Comitting code

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
- Find the process id of guardian
```
cat /var/vcap/sys/run/garden/garden.pid
```
- Enter guardian mount namespace
```
/var/vcap/packages/guardian/bin/inspector-garden -pid <pid> /bin/bash
```
- Go to container depot directory
```
cd /var/vcap/data/garden/depot/<handle>
```
- Execute process in container
```
/var/vcap/packages/runc/bin/runc exec <handle> <command>
```

### License

Apache License 2.0

