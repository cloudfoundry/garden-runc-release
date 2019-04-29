# Garbage Collection

## About

This document details how garbage collection of unused container image layers occurs inside Garden and GrootFS.
When using the default configuration, when a container is created in garden, GrootFS is invoked to fetch all required image layers and to combine them into a single container rootfs.
If creating a buildpack-based application, there is currently on a single layer - the Cloud Foundry stack (typically cflinuxfs2). The droplet (your application code + deps) is later copied into the rootfs.
There is work undergoing to also provide the droplet as a layer, resulting in the rootfs consisting of a 2 layer (the stack and the droplet) OCI image, but this can be ignored for now.
If creating a docker-based application, there are potentially many more layers!

It is desirable to keep these layers in a cache in order to improve the efficiency of future container creates, to avoid having to fetch layers again.
However, over time and with more and more creates, the cache can expand in size and start to take up a considerable amount of space on the `/var/vcap/data` disk.
Garbage collection in this context is the process by which we prune the cache of unused layers in order to free up disk space.

## What is an "unused" layer?

An unused layer is one that is not currently in use by any running container, and as such can be marked for garbage collection.

For example: Imagine that we create two containers, `Container A` and `Container B` from different base images:

```
- Container A
  Layers:
    - layer-1
    - layer-2
    - layer-3

- Container B
  Layers:
    - layer-1
    - layer-4
    - layer-5

```

They have a layer in common, `layer-1`. After deleting  `Container B`,
`layer-4` and `layer-5` are no longer in use, and as such can be deleted in order to free up disk space.

## When does garbage collection run?

Garbage collection occurs at the time of container creation, specifically just immediately after GrootFS has created the rootfs in order to avoid pruning the cache of layers that might be used by the container that's being created.
However, layers will only be deleted if a configured threshold has been exceeded. When determining if the threshold has been exceeded, the following things are taken into consideration:

1. Total disk size used by all layers, excluding the top level read-write layer presented to the container
1. The total disk size of all container disk quotas 

If the sum of these two things is >= the configured threshold, an attempt is made to clean up unused layers.

## How do I configure the garbage collection threshold?

To set the GC threshold, use the `grootfs.reserved_space_for_other_jobs_in_mb` BOSH property.
When this property is set, the threshold is calculated as follows:

```
threshold = sizeof(/var/vcap/data disk) in MB - (grootfs.reserved_space_for_other_jobs_in_mb)
```

In other words, GrootFS will use as much of the disk as possible, but will try to preserve at least `grootfs.reserved_space_for_other_jobs_in_mb` MB for other jobs to use. The hope is that this makes it easier for operators to configure the threshold to a value that actually makes sense! I.e. Rather than having to correctly guess a value to explicitly set the threshold value to.

**Note** If the value of `grootfs.reserved_space_for_other_jobs_in_mb` is > the size of the disk, then the threshold is set to 0, meaning that garbage collection will run on every container create.

**Note** Container creates will not fail if the new container would cause GrootFS to encroach into the reserved disk space, but garbage collection will be performed after creation.

## What value do you recommend I set grootfs.reserved_space_for_other_jobs_in_mb to?

If you are using cf-deployment, then this value will be set to a sane default for you. There shouldn't be a need to override it.
If you are not using cf-deployment, then you need to ensure you leave enough space for the diego executor cache, plus anything else that may be storing data on the `/var/vcap/data` disk. The size of diego executor cache is known in advance, and is configurable via the `diego.executor.max_cache_size_in_bytes` BOSH property. 

## How do I disable garbage collection entirely?

Garbage collection can be disabled by setting `grootfs.reserved_space_for_other_jobs_in_mb` to a value of `-1` (and ensuring that the deprecated properties are not set). 
