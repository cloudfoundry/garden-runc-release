# Migrating from garden-shed to GrootFS

In Garden 1.11.0, GrootFS become the default filesystem management tool used by
garden-runc. Unless setting the deployment property
`deprecated_use_garden_shed`, then deployments upgrading to versions >= 1.11.0
are using GrootFS as the filesystem management tool.

Although in most cases operators and app developers will not notice this change,
there are a few considerations that should be thought about.

This document is intended for operators and application developers on Cloud
Foundry and is applicable to cf-deployment versions 1.9.0+.

## Disk quota calculation

***garden-shed incorrectly enforces disk quotas by not considering the size of
docker images when creating the filesystem for an app.***

When pushing a cloud foundry app, one can specify a disk quota to enforce for
that app. For example, to set a disk limit of 200MB for an application, one
could say:

```
cf push myapp -k 200M -o my-docker-image
```

Imagine a docker image, who's layers unpacked to 300MB in size, which we will
call `docker-300` is pushed as a cf app. Then the following command under
garden-shed would succeed:

```
cf push myapp -k 200M -o my-dockerhub-account/docker-300
```

However, the same command run when GrootFS is enabled would result in an error
like the following:

```
2018-09-06T10:52:51.27+0100 [CELL/0] ERR Cell 84118b78-28ae-4f97-b0ec-35db7c043037 failed to create container for instance 67a8935c-ea21-4d8a-5ae6-7089: running image plugin create: pulling the image: layers exceed disk quota 316734553/209715200 bytes
2018-09-06T10:52:51.27+0100 [CELL/0] ERR : exit status 1
```

This can be mitigated by checking your disk usage beforehand. You should check
that, for each application, the disk usage by an app + the usage of the
unpacked* docker image does not exceed the disk quota for that app. If so, the
app's disk quota should be increased prior to migrating to GrootFS.

\* *Docker images are stored in an archived format, and their size after being
unpacked on the Diego cell may be larger than their packed size.*

### Notes

* The disk quota for existing apps can be observed by looking at the "disk"
column of the `cf apps` output.
* The unpacked docker image size can be inspected by looking under the "SIZE"
column of the out of `docker images`.
* To see current disk utilisation, use `cf app my-app`. If the in-use disk is
larger than the available disk for any app then that apps quota should be
increased before migrating to GrootFS.

## garden-shed debris left on diego-cells

***GrootFS and garden-shed store filesystem layers in different directories,
and there may be less disk space available until the cells are recreated.***

garden-shed would store filesystem layers on the diego cell under
`/var/vcap/data/garden/graph` and GrootFS stores them under
`/var/vcap/data/grootfs/store`. Since GrootFS doesn't know how to cleanup unused
layers that were created by garden-shed, those layers will remain untouched on
the diego-cell. This shouldn't be a problem unless the cell is approaching using
all of its disk. In order to reclaim this space, operators can recreate the
diego-cell or coincide the GrootFS migration with a stemcell bump.
