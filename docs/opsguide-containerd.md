### Terminology

When it is running in containerd mode, Guardian will delegate all container operations to containerd.
This means that containerd is doing all the housekeeping and Guardian is just acting as a client. That's
why it is important to understand how Guardian is mapping its terminology to the terminology of containerd.
Here is a table describing that mapping.

```
|---------------|-----------------|--------------------------------------------------------------|
| Guardian Term | Containerd Term | RunC Representation                                          |
|---------------|-----------------|--------------------------------------------------------------|
| OCI Bundle    | Container       | The `config.json` used by RunC                               |
| Container     | Task            | The running instance of a RunC bundle (`runc run`)           |
| Process       | Process         | A process `exec`-ed into a running RunC bundle (`runc exec`) |
|---------------|-----------------|--------------------------------------------------------------|
```

Another important concept of containerd is the concept of `namespaces`. Each client can work in its own
namespace, so that different clients do not have to give their containers globally unique names. Guardian
is working in a namespace named "garden".

#### The ctr client
Containerd has a command line client named `ctr` that you can use in order to list and inspect containers
created by Guardian in containerd mode. It is important to use the right containerd socket and namespace.
Here is how to list all the containers (a.k.a tasks in containerd terms):

```
/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden tasks ls
```

Look at [ctr docs](https://github.com/projectatomic/containerd/blob/master/docs/cli.md) for more info.

#### Processes

For the time being the containerd mode is limited to container operations. So even when containerd mode
is on, all process operations are still being handled by "RunDmc" - the RunC mode containerizer. Please
have a look at the [RunC mode ops guide](opsguide-runc.md#processes) for more details.

#### The Bundle Config
If you need to look at your RunC bundle config it can be found at:

```
/var/vcap/data/containerd/state/io.containerd.runtime.v1.linux/garden/$containerid/config.json
```

You can also use the `ctr` client:

```
/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden containers info $containerid
```

## Containerizer

RunContainerd is Guardian's containerd mode containerizer. It is a thin wrapper around containerd.
In order to create and manage containers it relies on a containerd instance running on a unix socket.
The socket path is `/var/vcap/sys/run/containerd/containerd.sock`

## Interacting with a Container

As Guardian uses containerd to create containers you can use the `ctr` client to interact with them for debug purposes.
For example, to execute a process in a container you can execute:

```
/var/vcap/packages/containerd/bin/ctr -a /var/vcap/sys/run/containerd/containerd.sock -n garden tasks exec --exec-id my-shell --tty $containerid /bin/sh
```

## Further Reading

To enable the usage of containerd:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/spec#L212-L214

In addition, in order to enable containerd for container process management:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/spec#L231-L233

For Containerd's daemon, we set up the config file here:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/spec#L22

Containerd's config file is built here:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/templates/config/containerd.toml.erb#L28-L51

Garden Start starts up the containerd process by calling the method `start_containerd`:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/templates/bin/garden_start.erb#L46

Which then kicks off the steps to invoke containerd with the previously generated config file:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/templates/bin/containerd_utils.erb#L28-L68

Garden's start had previously been accomplished by the configuration of this BPM configuration file:
https://github.com/cloudfoundry/garden-runc-release/blob/861e755/jobs/garden/templates/config/bpm.yml.erb#L1C1-L8C22
