# Enabling rootless containers

[Rootless containers](docs/articles/rootless-containers.md) improve the security of a
Garden deployment by limiting the privilege of the Garden server. However,
there are a few prerequisites and consequences to enabling this feature, which
this document will describe. We'll be focussing on Garden in the context of
Cloud Foundry, unless otherwise noted.

## Privileged Containers

In Garden nomenclature, a "privileged container" is a container with no new
[user namespace](http://man7.org/linux/man-pages/man7/user_namespaces.7.html)
created for it. Users (including root) in the container processes are not
mapped in any way.  Therefore if a process runs as root in a privileged
container, it is running as "real" root with all associated capabilities.

Unprivileged containers are far more secure than privileged ones, as a
privilege escalation exploit that grants a malicious user the ability to run
code as root will not actually result in all the Linux capabilities of root
being granted to that user. For example, they will not be able to `mknod` block
devices or mount most filesystems, which would allow them to explore the host's
root filesystem, and the filesystems of other containers.

A Garden server that runs as root can create privileged containers [on
request](https://godoc.org/code.cloudfoundry.org/garden#ContainerSpec). You
can't use privileged containers if you enable the rootless containers feature,
because the server can't spawn processes with more privilege than it already
has!

Cloud Foundry app instances used to run in privileged containers by default,
mainly so that
[FUSE](https://www.kernel.org/doc/Documentation/filesystems/fuse.txt) could be
used by applications. This is now neither [the default setting in Cloud
Controller](http://bosh.io/jobs/cloud_controller_ng?source=github.com/cloudfoundry/capi-release&version=1.45.0#p=cc.diego.use_privileged_containers_for_running),
nor is it set in
[cf-deployment](https://github.com/cloudfoundry/cf-deployment/blob/master/cf-deployment.yml). 

However, even if Cloud Foundry is configured not to run or stage apps in
privileged containers, older applications that were deployed while privileged
containers were still the default will continue to be run in privileged
containers, even when instances are started/stopped by Diego for any reason.

### Preventing future apps from running in privileged containers

You should only perform these steps if none of your apps require privileged
containers.

If you are not using the CC bridge components (your deployment manifest does
not contain the "stager" or "nsync" jobs), then ensure that your deployment
manifest doesn't set `cloud_controller_ng`'s `use_privileged_containers_for_running`
or `use_privileged_containers_for_staging` properties to `true`. See [the bosh
job
spec](http://bosh.io/jobs/cloud_controller_ng?source=github.com/cloudfoundry/capi-release&version=1.45.0#p=cc.diego.use_privileged_containers_for_running)
(this is the latest version at the time of writing).

If you do use the CC bridge components, ensure
[`nsync`'s](http://bosh.io/jobs/nsync?source=github.com/cloudfoundry/capi-release&version=1.45.0#p=capi.nsync.diego_privileged_containers)
and
[`stager`'s](http://bosh.io/jobs/stager?source=github.com/cloudfoundry/capi-release&version=1.45.0#p=capi.stager.diego_privileged_containers)
`diego_privileged_containers` properties are not set to true.

### Determining if any app instances are privileged

On up-to-date deployments, the [cfdot](https://github.com/cloudfoundry/cfdot)
will be available on the diego-cell VMs. Run `cfdot desired-lrps | jq
.privileged` to determine if any app instances are running in privileged
containers.

### Converting privileged app instances to unprivileged

Firstly, does the app require privileged features? The most common reason apps
need to run in privileged containers is to use FUSE filesystems, e.g. sshfs.
If you aren't sure, push (or ask whoever is responsible for the app to push)
the app to a test org and space under a different name. If you've disabled
privileged containers for new apps, then this new instance will be
unprivileged. If the app still works, you can proceed.

To convert the privileged app to unprivileged, run `cf restage <app name>`.

## Enabling rootless containers

Set the `garden.experimental_rootless_mode` to true on the garden job, then
re-deploy with `--recreate`. Upgrading without recreating the VMs is not
supported, and probably won't work.
