## Important Directories

### The Depot

Guardian stores container bundles in the standard OCI format underneath a directory called "the depot". 

If you deployed guardian using garden-runc-release the depot is located at `/var/vcap/data/garden/depot`. 

#### The Bundle Config

Each subdirectory of the depot describes a single container in a format known as an "OCI Bundle". This is essentially a directory containing an OCI-format `config.json` which is the configuration passed to `runc create` to create a container. 

### Processes

#### Process directories

For each process created inside the container (via the garden `container.Run` API) a subdirectory is created in `$depot_dir/$container_handle/processes/$process_guid`.

Inside this directory are named pipes (FIFOs) representing the stdin/out/err streams, a pipe named ‘exit’ which remains open while the process is open and then is closed when it exits and a named pipe named ‘winsz’ which is used to communicate window size change events to the running process. These files are connected to the container process by the `dadoo` helper executable (described below), allowing guardian to re-connect to the process’s streams and wait for the process to exit even after restarting.

#### Processes with with overriden Images/Limits

Many container processes share everything with other processes in the same
container. At the runC level, these are also modelled as processes "inside" a
container, and can be listed using `runc ps`. However, using the fields `Image`
or `OverrideContainerLimits` on a `ProcessSpec`, processes that do not share
everything with other processes in the same container can be created.

At the runC level, these "partially unshared" Garden processes are not runC
processes, but runC containers. This separate container will still share many
of its resources with the initial runC container that was created when we
created the Garden container that this process is a member of.

Like the other runC containers, these process containers also have a bundle
directory. It is located in
`$depot_dir/$outer_container_handle/processes/$inner_container_dir/` and is a
combination of the `Container subdirectories` and the `Process subdirectories`
described above. The directory contains a `config.json` which describes the
"side container" that is going to be created when calling `runc run -d` and
also the pipes and `pidfile` of the process that is going to run in that
container.

The newly created container can be seen by running `runc list`, and all
container-related operations apply to it as usual. Using its id, you can get
information about the partially-unshared process through `runc ps <id>`.

## Containerizer

RunDMC is Guardian’s runc mode containerizer. It is a super-small wrapper around runc with two jobs. Firstly it manages the ‘depot’ directory (see above), creating, listing and destroying container bundles as requested (in the code this is the `DirDepot` component). Secondly it executes `runc` commands to actually create, run and manage the container lifecycle (in the code this is the `RuncRunner` component).

### Dadoo

At runtime, you will see that guardian itself runs as a daemon process to which your client connects (normally over a unix socket). For each running container, guardian uses the `runc run` command against a particular bundle directory in the depot dir to create a container. This container immediately runs guardian's “init” process which makes sure that the container will not leak zombie processes. As processes are run inside the container (for example user apps), guardian spawns a process named `dadoo` which then calls `runc exec` to run the requested process (in other words, [dadoo runs runC](https://www.youtube.com/watch?v=dqgtsai2aKY)). Dadoo keeps running as long as the user’s process runs, monitoring its exit status and eventually writing it to a file in the `processes/$processid` subdir. If guardian is restarted it is able to reconnect to the running process via dadoo.

## Interacting with a Container

At runtime, bundles from the depot directory are run using [runC](http://github.com/opencontainers/runc). As these are standard runc containers and processes, you can interact with them for debug purposes using runc itself (or any compatible tool). For example, to execute a process in a container you can execute:

```
/var/vcap/packages/runc/bin/runc exec $containerid /bin/sh
```

(where $containerid is the directory name of the depot subdirectory in question). To get events from a container you can run 
```
/var/vcap/packages/runc/bin/runc events $containerid
``` 

and so on. More documentation on runC is available [on its github page](http://github.com/opencontainers/runc).
