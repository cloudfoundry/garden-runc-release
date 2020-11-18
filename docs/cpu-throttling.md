# CPU Throttling

Enable cpu throttling by setting the
[experimental_cpu_throttling](https://github.com/cloudfoundry/garden-runc-release/blob/42497fc3b1365210ffa9681acf97f3f7314bbd01/jobs/garden/spec#L159-L161)
flag to true in the garden config.

For details of the feature, see this [blog
post](https://www.cloudfoundry.org/blog/better-way-split-cake-cpu-entitlements/)
and this [video](https://youtu.be/vV87xmxKLeA).

## Enabling and disabling cpu-throttling

Ensure you set the [destroy_containers_on_start](https://github.com/cloudfoundry/garden-runc-release/blob/42497fc3b1365210ffa9681acf97f3f7314bbd01/jobs/garden/spec#L171-L173) flag when changing this property, otherwise existing containers will be in the wrong part of the cgroup structure.

### cgroup structure

When enabled, two extra cpu cgroup directories are inserted in the garden
cgroup directory structure. App containers begin in the good cgroup subtree,
and if their cpu usage allowance is exceeded, they are moved to the bad cgroup
subtree. These directories are not present used when cpu-throttling is
disabled. When enabling or disabling this feature there are two scenarios we
want to avoid:

#### 1. Switching from *no* throttling to throttling

In this case, if we have two apps - A and B (with 512 and 256 shares
respectively), the cgroup structure would look like this:

```
garden - 1024
└ A - 512
└ B - 256

```

When we redeploy *with* throttling this will create two more cgroups under
the garden one - good and bad. Any new containers will be put in the `good`
cgroup upon creation, however, the old containers will **not** be moved. In
this case, with a new container -D (with 256 shares), the structure would look
like this:

```
garden - 1024
└ good - 1022
  └ D - 256
└ bad - 2
└ A - 512
└ B - 256
```

Obviously this is a problem, since it would mean that the sum of good + bad
cgroups would not yield 100% of the cpu.

#### 2. Switching from throttling to *no* throttling:

Lets say we have two apps - A and B and one of them is currently in the bad
cgroup:

```
garden - 1024
└ good - 768
  └ A - 512
└ bad - 256
  └ B - 256
```

If we were to redeploy with cpu throttling disabled, this means that the app
will never leave the bad cgroup and will always be throttled.

