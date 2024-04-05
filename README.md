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
See the [Contributing.md](./.github/CONTRIBUTING.md) for more information on how to contribute.

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
