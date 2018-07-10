# How much disk space is GrootFS using?

Understanding how GrootFS uses its disk, and how to find out about that usage, is important for operators or support agents who are reconciling disk allocations for containers with actual disk usage on the host.
This is often complicated by the fact that standard tools (`du` and `df`) can be misleading due to how container filesystems work.

Eg:
- Operator observes that the Diego cell rep is out of disk capacity to allocate, but actual disk usage on the Garden host is relatively low, resulting in containers not being placed on the cell.
- Operator observes that, conversely, the Diego cell rep thinks it has disk space to allocate, but containers and system components together are using enough disk that the remaining available
  disk space is less than the allocation, resulting in containers continuing to be placed on the cell and then failing to start.


These commands are designed to be run on a BOSH deployed VM running the garden job.

## How much disk is used exclusive to container X?
`grootfs stats` returns 2 values: `total_bytes_used` and `exclusive_bytes_used`. `exclusive` is the disk usage of the container **not** including the rootfs image layers,
and `total` is inclusive of those layers.

```sh
$ ls /var/vcap/data/garden/depot/ # look up container handles
057713a4-8939-4d1a-5b95-e89a  55afbf65-5cbf-49c6-4461-f803

$ /var/vcap/packages/grootfs/bin/grootfs --config /var/vcap/jobs/garden/config/grootfs_config.yml stats 55afbf65-5cbf-49c6-4461-f803
{"disk_usage":{"total_bytes_used":23448093,"exclusive_bytes_used":8192}}
```

If you believe you are also creating privileged containers, update the config path in the above command to use `privileged_grootfs_config.yml`.

## How much exclusive disk is used across all running containers?
```sh
$ ls /var/vcap/data/garden/depot/ \
    | xargs -I{} /var/vcap/packages/grootfs/bin/grootfs --config \
    /var/vcap/jobs/garden/config/grootfs_config.yml stats {}  \
    | cut -d: -f4 | cut -d} -f1 | awk '{sum += $1} END {print sum}'
```
If you believe you are also creating privileged containers, update the config path in the above command to use `privileged_grootfs_config.yml`.

## How much disk does underlying layer Y (rootfs or, in future, droplet or other non-root layer) use?
Underlying layers are known as `volumes` in GrootFS. They are read-only and their changesets are layered together through an `overlay` mount to create the rootfs for containers.
When GrootFS writes each filesystem layer/volume to disk, it also stores the number of bytes written to a file in `meta`. To find out the size of an individual layer/volume, we just need to
read the corresponding metadata file.

```sh
$ cat /var/vcap/data/grootfs/store/unprivileged/meta/volume-<abcd-layer-sha>
{"Size":5607885} # unit in bytes
```
We can also use `du` (passing the absolute path to the volume) to find out the same thing.
```sh
$ du -sch /var/vcap/data/grootfs/store/unprivileged/volumes/<abcd-layer-sha>/
5.4M    /var/vcap/data/grootfs/store/unprivileged/volumes/<abcd-layer-sha>/
```
If you believe you are also creating privileged containers, update the store paths in the above command.

## How much disk do all the active layers use in total?
For each container, GrootFS mounts the underlying volumes using `overlay` to a point in the `images` directory. This point is the rootfs for the container and is read-write.
GrootFS also stores the shas of each underlying volume used by an image in the `meta` folder. To find out which layers/volumes are active, we first parse all the dependency metadata
for every image (container rootfs), remove the duplicates, and then read each volume's metadata file (as we did in the section above). Finally we can sum the bytes of all active volumes on disk.

```sh
$ for image in $(ls /var/vcap/data/grootfs/store/unprivileged/meta/dependencies/image\:*.json | awk '/!store/'); \
    do cat $image | python -c 'import json,sys;obj=json.load(sys.stdin); \
    print "\n".join(obj)' ; done | sort -u | xargs -I{} cat /var/vcap/data/grootfs/store/unprivileged/meta/volume-{} | cut -d : -f 2 | cut -d} -f1 \
    | awk '{sum += $1} END {print sum}'
```
If you believe you are also creating privileged containers, update the store paths in the above command.

## How much disk does the store use in total?
```sh
$ df | grep -E  "/var/vcap/data/grootfs/store/(privileged|unprivileged)$" \ | awk '{sum += $3} END {print sum}'
```

## How much disk could be reclaimed through pruning unused layers?
You can use values gathered from commands above to calculate how much space could be reclaimed by pruning unused layers: total store disk usage - active layers.

## Are there categories of grootfs disk usage that should be accounted for, other than the ones above? If so, how much disk usage do they account for?
The bulk of disk usage will go to the `images` and `volumes` directories (found under `/var/vcap/data/grootfs/store/{unprivileged,privileged}/`).


GrootFS also stores information in the following directories:
 - `l` -> link dirs. Shorter dir-names symlinked to volume dirs to allow groot to union mount more layers/filepaths.
 - `locks` -> filesystem lock dir to ensure safety during concurrent cleans/creates.
 - `meta` -> per image and volume metadata.
 - `projectids` -> empty numbered directories used to track image quotas.
 - `tmp` -> normal tempdir stuff.


These would would usually total less than 2MB.

## If I force grootfs to prune its cache, would I be able to reduce total disk usage to Z% of local disk or to W GB?
It would rarely be necessary to force GrootFS to clean its cache, but if more space is needed on disk, then operators should set the `reserved_space_for_other_jobs_in_mb` property to a higher value.
How much space is freed would depend on how many layers/volumes are actively used by images/rootfses. You can use values gathered from commands above to calculate the space which
could be cleared by a force clean: total disk in use by store - total disk used by active layers.

## Is grootfs using more disk than it is configured to use, or is it behaving correctly?
Grootfs stores are [initialised](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/jobs/garden/templates/bin/overlay-xfs-setup#L23-L46) to use the [entire](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/jobs/garden/templates/bin/grootfs-utils.erb#L31) `/var/vcap/data`. If the `reserved_space_for_other_jobs_in_mb` is not set high enough, or if there are many images with few shared volumes, it could use it all.
The [Thresholder](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/src/thresholder/main.go) calculates and sets a value so that Groot’s GC will try to ensure that a small reserved space is kept free for other jobs. Groot will only try to GC when that threshold is reached.
However, if all the rootfs layers are active (in use by images), then GC cannot occur and that space will be used.

