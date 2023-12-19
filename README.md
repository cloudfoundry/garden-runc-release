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

See [the `bosh-lite` deployment wiki
page](https://github.com/cloudfoundry/garden-runc-release/wiki/Creating-sandbox-environments-for-debugging#local-bosh-lite-deployment)

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

- Clone [CI repository](https://github.com/cloudfoundry/wg-app-platform-runtime-ci) (next to where this code is cloned), and make sure latest
is pulled by running `git pull`

  ```bash
  mkdir -p ~/workspace
  cd ~/workspace
  git clone https://github.com/cloudfoundry/wg-app-platform-runtime-ci.git
  ```

In order to contribute to the project you may want some of the following installed:

- [Git](https://git-scm.com/) - Distributed version control system
- [Go](https://golang.org/doc/install#install) - The Go programming
  language
- [Fly CLI](https://github.com/concourse/fly) - Concourse CLI
- [Virtualbox](https://www.virtualbox.org/) - Virtualization box
- [Vagrant](https://www.vagrantup.com/) - Portable dev environment

Garden-runC uses git submodules to maintain its dependencies and components.
Some of Garden-runC's important components currently are:

- [Garden](https://github.com/cloudfoundry/garden) found under
  `src/garden` is the API server and client.
- [Guardian](https://github.com/cloudfoundry/guardian) found under
  `src/guardian` is the Garden backend.
- [GrootFS](https://github.com/cloudfoundry/grootfs) found under
  `src/grootfs` downloads and manages
  root filesystems.
- [GATS](https://github.com/cloudfoundry/garden-integration-tests)
  found under `src/garden-integration-tests`
  are the cross-backend integration tests of Garden.

### Running the tests with fly

[Concourse CI](https://concourse-ci.org/) is used for running Garden-runC tests
in a VM. It provides the [Fly CLI](https://github.com/concourse/fly) for
Linux and MacOSX. Instructions for deploying a single VM Concourse using BOSH
can be found in the [concourse-deployment repo](https://github.com/concourse/concourse-deployment)

Once running, navigate to [https://192.168.100.4:8080](https://192.168.100.4:8080) in a web browser
and download the [Fly CLI](https://concourse-ci.org/download.html) using the links found in
the bottom-right corner. Place the `fly` binary somewhere on your `$PATH`.

- `./scripts/create-docker-container.bash`: This will create a docker container with mounts for running linter and templates tests
- `./scripts/fly/build-binaries.bash`: This will build binaries required for testing garden-runc-release
- `./scripts/fly/test.bash`: This will test a specific package e.g. './scripts/fly/test.bash grootfs'
- `./scripts/fly/test-all.bash`: This will test all packages.
- `./scripts/test-in-docker-locally.bash`: This will only test templates and linters

When inside docker container: 
- `/repo/scripts/docker/tests-templates.bash`: This will run all of tests for bosh tempalates
- `/repo/scripts/docker/lint.bash`: This will run all of linting defined for this repo.

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

Commit the changes, run the tests, and create a bump commit.

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

### Maintainers

See the list of [MAINTAINERS](docs/maintainers.md) and their contact info.

### License

Apache License 2.0
