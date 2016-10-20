# Transitioning from garden-linux-release to garden-runc-release

The transition from garden-linux to garden-runc is fairly simple.
All you need to do is upload the [garden-runc-release](http://bosh.io/releases/github.com/cloudfoundry/garden-runc-release?all=1) to your BOSH director, swap out `garden-linux-release` for `garden-runc-release` in your deployment manifest and then bosh deploy.
You shouldn't need to make any property changes.

However, please note the following before running the deploy:

1. You *MUST* make sure that any VMs running garden-linux (namely the cell VMs in a CF deployment) get recreated during the deployment. This can be achieved in one of two ways:
  1. Bump the stemcell for the deployment.
  1. Run the deployment with the `--recreate` flag, e.g. `bosh deploy --recreate`.
1. If you are using the [diego manifest generation scripts](https://github.com/cloudfoundry/diego-release/blob/develop/docs/manifest-generation.md#-g-opt-into-using-garden-runc-release-for-cells), you can pass the `-g` flag to the `generate-deployment-manifest` script in order to generate a manifest with garden-runc-release instead of garden-linux-release.

