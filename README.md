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

**Note**: If you are transitioning from garden-linux-release to
garden-runc-release, please read the transition doc
[here](https://github.com/cloudfoundry/garden-runc-release/blob/master/docs/transition-from-garden-linux.md).

The easiest way to run Garden-runC is to deploy it with [BOSH
Lite](https://bosh.io/docs/bosh-lite.html), a VirtualBox development
environment for [BOSH](https://bosh.io). Once you have  set up bosh-lite
(follow the instructions in the bosh-lite repo), just deploy like any bosh
release, e.g:

~~~~
cd garden-runc-release # if you're not already there
./scripts/create-upload-lite-release.sh
./scripts/deploy-lite.sh
~~~~

You can retrieve the address of the Garden-runC server by running `bosh vms`.
It will be `10.244.16.6` if using the provided bosh-lite manifest in
[manifests/bosh-lite.yml](https://github.com/cloudfoundry/garden-runc-release/blob/master/manifests/bosh-lite.yml).
The server port defaults to `7777`.

### Usage

The easiest way to start creating containers is to use the
[`gaol`](https://github.com/contraband/gaol) command line client.

e.g. `gaol -t 10.244.16.6:7777 create -n my-container`

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

In order to contribute to the project you should have the following installed:

- [Git](https://git-scm.com/) - Distributed version control system
- [Go](https://golang.org/doc/install#install) - The Go programming
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
Linux and MacOSX. A Single VM Concourse can be deployed using BOSH.
Create a new directory:

```bash
mkdir concourse-lite
cd concourse-lite
```

Then follow the instructions [here](http://concourse.ci/concourse-lite.html)

Then open [http://192.168.100.4:8080](http://192.168.100.4:8080) in a web browser
and download the [Fly CLI](http://concourse.ci/fly-cli.html) using the links at
the bottom-right corner. Place the `fly` binary somewhere on your `$PATH`.

The tests use the [Ginkgo](https://onsi.github.io/ginkgo/) BDD testing
framework.

Assuming you have configured a Concourse and installed Ginkgo, you can run all
the tests by executing `./scripts/test`.

Note: The concourse-lite VM may need to be provisioned with more RAM
If you start to see tests failing with 'out of disk' errors,  open the
`concourse-lite.yml`, change the properties detailed below, and [recreate the env](http://concourse.ci/concourse-lite.html). 

```
resource_pools:
- cloud_properties:
    cpus: 4
    ephemeral_disk: 32768
    memory: 6144
```

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
executing `./scripts/test`.
To run individual tests, use`./scripts/remote-fly`:

```bash
# Running Guardian tests
./scripts/remote-fly ci/guardian.yml

# Running Garden tests
./scripts/remote-fly ci/garden.yml

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
debuggging issues on garden-runc-release deployments. Run this command on the
deployment VM as root:

`curl bit.ly/garden-ordnance-survey -sSfL | bash`

### License

Apache License 2.0
