# Garbage Collection

## About

This document details how garbage collection of unused container image layers occurs inside Garden and Grootfs.
When a container is created in garden, Grootfs is invoked to download all required image layers and to combine them into a single container rootfs.
If creating a buildpack-based application, there are only 2 layers - the rootfs layer (typically cflinuxfs2) and the droplet layer (your appliaction code + deps).
If creating a docker-based application, there are potentially many more layers!

It is desirable to keep these layers in a cache in order to improve the efficiency of future container creates.
However, over time and with more and more creates, the cache can expand in size and start to take up a considerable amount of space on the `/var/vcap/data` disk.
Garbage collection in this context is the process by which we prune the cache of unused layers in order to free up disk space.

## What is an "unused" layer?

An unused layer is one that is not currently in use by any running container, and as such can be marked for garbage collection.

For example: Imagine that we create two containers from different base
images, `Container A` and `Container B`:

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

Garbage collection occurs at the time of container creation, specifically just immediately after Grootfs has created the rootfs. This is to avoid pruning the cache of layers that might be used by the container that's being created.
However, layers will only be deleted if the configured threshold has been reached. When determining if the threshold has been reached, the following things are taken into consideration:

1. Total disk size used by all layers on disk
1. The total disk size of all container disk quotas 

If the sum of these two things is >= the configured threshold, then we attempt to clean up unused layers.

## How do I configure the garbage collection threshold?

There are 3 key BOSH properties you need to be aware of:

1. `grootfs.reserved_space_for_other_jobs_in_mb`
1. `garden.graph_cleanup_threshold_in_mb` (DEPRECATED)
1. `grootfs.graph_cleanup_threshold_in_mb` (DEPRECATED)

The recommended way to set the threshold is to use the `grootfs.reserved_space_for_other_jobs_in_mb` BOSH property.
When this property is set (and assuming the two deprecated properties are not set), the threshold is calculated as follows:

```
threshold = size of the /var/vcap/data disk - the value of grootfs.reserved_space_for_other_jobs_in_mb
```

In other words, Grootfs will use as much of the disk as possible, but will try to preserve at least `grootfs.reserved_space_for_other_jobs_in_mb` MB for other jobs to use. The hope is that this makes it easier for operators to configure the threshold to a value that actually makes sense! I.e. Rather than having to correctly guess a value to explicitly set the threshold value to.

**Note** If the value of `grootfs.reserved_space_for_other_jobs_in_mb` is > the size of the disk, then the threshold is set to 0, meaning that garbage collection will run on every container create.

**Note** Container creates will not fail if the new container would cause Grootfs to encroach into the reserved disk space.

The `garden.graph_cleanup_threshold_in_mb` and `grootfs.graph_cleanup_threshold_in_mb` properties cause the threshold to be set explicitly. These are now marked as deprecated and only exist in order to preserve backwards compatibility. We recommend that you stop setting these properties entirely, and instead set the threshold implicitly via the `grootfs.reserved_space_for_other_jobs_in_mb`. 

## What value do you recommend I set grootfs.reserved_space_for_other_jobs_in_mb to?

If you are using cf-deployment, then this value will be set to a sane default for you. There shouldn't be a need to override it.
If you are not using cf-deployment, then you need to ensure you leave enough space for the diego executor cache, plus anything else that may be storing data on the `/var/vcap/data` disk. The size of diego executor cache is known in advance, and is configurable via the `diego.executor.max_cache_size_in_bytes` BOSH property. 

## How do I disable garbage collection entirely?

Garbage collection can be disabled by setting `grootfs.reserved_space_for_other_jobs_in_mb` to a value of `-1` (and ensuring that the deprecated properties are not set). 
