# How much disk space is GrootFS using?

Understanding how GrootFS uses its disk, and how to find out about that usage, is important for operators or support agents who are reconciling disk allocations for containers with actual disk usage on the host.
We have seen some situations where naively using the standard tools (`du` and `df`) to discover this doesn't work as expected.

Eg:
- Operator observes that the Diego cell rep is out of disk capacity to allocate, but actual disk usage on the Garden host is relatively low, resulting in containers not being placed on the cell.
- Operator observes that, conversely, the Diego cell rep thinks it has disk space to allocate, but containers and system components together are using enough disk that the remaining available
  disk space is less than the allocation, resulting in containers continuing to be placed on the cell and then failing to start.


These commands are designed to be run on a BOSH deployed VM running the garden job.

## How much disk is used exclusive to container X?
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
```sh
$ cat /var/vcap/data/grootfs/store/unprivileged/meta/volume-<abcd-layer-sha>
{"Size":5607885}
```
or
```sh
$ du -sch /var/vcap/data/grootfs/store/unprivileged/volumes/<abcd-layer-sha>/
5.4M    /var/vcap/data/grootfs/store/unprivileged/volumes/<abcd-layer-sha>/
```
If you believe you are also creating privileged containers, update the store paths in the above command.

## How much disk do all the active layers use in total?
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
Total store disk usage - active layers

## Are there categories of grootfs disk usage that should be accounted for, other than the ones above? If so, how much disk usage do they account for?
The following metadata is stored:
 - l = link dirs. Short dir-names symlinked to volume dirs to allow groot to union mount more layers/filepaths.
 - locks = filesystem lock dir
 - meta = per image and layer metadata
 - projectids = empty numbered directories used to track image quotas
 - tmp = normal tempdir stuff
This would usually total less than 1MB so it is not worth worrying about.

## If I force grootfs to prune its cache, would I be able to reduce total disk usage to Z% of local disk or to W GB?
This depends on how many images the cell has and how many layers those images have in common. You can use values gathered from commands above to calculate the space which
could be cleared by a force clean: Total disk in use by store - total disk used by active layers.

## Is grootfs using more disk than it is configured to use, or is it behaving correctly?
Grootfs stores are [initialised](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/jobs/garden/templates/bin/overlay-xfs-setup#L23-L46) to use the [entire](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/jobs/garden/templates/bin/grootfs-utils.erb#L31) `/var/vcap/data`. So if it uses it all, then yes it is behaving correctly.
The [Thresholder](https://github.com/cloudfoundry/garden-runc-release/blob/b4a44c5cabb1570eaeb25b158823cfbd97ae530c/src/thresholder/main.go) calculates and sets a value so that Groot’s GC will try to ensure that a small reserved space is kept free for other jobs. Groot will only try to GC when that threshold is reached.
However, if all the rootfs layers are active (in use by images), then GC cannot occur and that space will be used.



