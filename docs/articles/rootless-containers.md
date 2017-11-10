# Rootless containers in Garden

At the end of March 2017, a [long-running
PR](https://github.com/opencontainers/runc/pull/774) to add support for
"rootless" containers to runC was finally merged into runC master. Since then,
the Cloud Foundry Garden team have been working to expose this functionality at
the Garden level. Although only experimental at the moment, Garden's support
for rootless containers is now at a stage where we feel ready to talk a little
bit more about what we've done and where we're headed.

But before getting into all that, what exactly is a "rootless" container? Put
simply, it is one that does not require any root privileges to create and run.
This of course has huge potential for adoption of containers in situations
where historically this may not have been possible. The driving use cases at
the moment seem mostly to be academic ones, however there are also security
benefits to be gained by not including the root user at any point of a
container's life cycle.

# Component overview

Garden makes use of a number of components in order to provide rootless
support. These components are configured as plugins that are passed to the
Garden server at startup. There are 3 plugins to be specific - Runtime, Network
and Image.

* Runtime Plugin - [runC](https://github.com/opencontainers/runc) - A CLI tool
  for spawning and running containers according to the OCI specification.
* Network Plugin - [silk CNI](https://github.com/cloudfoundry-incubator/silk) -
  an open-source, CNI-compatible container networking fabric.
* Image Plugin - [grootfs](https://github.com/cloudfoundry/grootfs) - A
  daemonless container image manager.

Both runC and grootfs provide rootless support out of the box. The garden
server (aka `gdn`) is also able to run as a non-root user. Networking is a
little more difficult... and so right now we have to rely on the setuid bit for
the networking binaries.

## Step-by-Step installation

The rest of this article details step-by-step instructions for installing,
configuring and running Garden as a non-root user on an Ubuntu Xenial machine.
For this guide, we assume commands are run as the `ubuntu` user, however you
should be free to use any non-root user you like (subject to the prerequisites
below).

If you run into any issues along the way, feel free to chat to us on the
`#garden` channel of the [Cloud Foundry Slack](http://slack.cloudfoundry.org/).

### Prerequisites

* An Ubuntu Xenial machine (with kernel version 4.4+)
* An entry for the non-root user in the `/etc/sub{u,g}id` files
* Write permissions for the non-root user on a directory at `/var/run/user/$(id -u)`
* sudo permissions (required only for initial setup)

All prerequisites are met by default for the ubuntu user.

### Step 1: Download binaries and set permissions

* [gdn](https://github.com/cloudfoundry/garden-runc-release/releases/download/v1.9.5/gdn-1.9.5)
* [grootfs](https://github.com/cloudfoundry/grootfs/releases/download/v0.28.1/grootfs-0.28.1)
* [cni](https://github.com/containernetworking/cni/releases/download/v0.5.0/cni-amd64-v0.5.0.tgz)
* [garden-external-networker](https://s3.amazonaws.com/garden-external-networker/garden-external-networker)

```
GDN_DOWNLOAD_LINK=https://github.com/cloudfoundry/garden-runc-release/releases/download/v1.9.5/gdn-1.9.5
GROOTFS_DOWNLOAD_LINK=https://github.com/cloudfoundry/grootfs/releases/download/v0.28.1/grootfs-0.28.1
CNI_DOWNLOAD_LINK=https://github.com/containernetworking/cni/releases/download/v0.5.0/cni-amd64-v0.5.0.tgz
EXTERNAL_NETWORKER_DOWNLOAD_LINK=https://s3.amazonaws.com/garden-external-networker/garden-external-networker

mkdir -p $HOME/gdn/{assets,bin,config,garden-cni/config}

wget -qO $HOME/gdn/bin/gdn $GDN_DOWNLOAD_LINK
wget -qO $HOME/gdn/bin/grootfs $GROOTFS_DOWNLOAD_LINK
wget -qO- $CNI_DOWNLOAD_LINK | tar -xz -C $HOME/gdn/bin/
wget -qO $HOME/gdn/bin/garden-external-networker $EXTERNAL_NETWORKER_DOWNLOAD_LINK

chmod u+x $HOME/gdn/bin/{gdn,grootfs}
sudo chown root:root $HOME/gdn/bin/{host-local,bridge,garden-external-networker}
sudo chmod 4755 $HOME/gdn/bin/{host-local,bridge,garden-external-networker}
```

### Step 2: Configure the image and network plugins

```
curl -s https://raw.githubusercontent.com/cloudfoundry/grootfs/v0.28.1/hack/quick-setup | sudo bash
sudo chown -R $(id -u):$(id -u) /var/lib/grootfs/btrfs

cat > $HOME/gdn/config/garden-cni-config.json <<EOF
{
  "cni_plugin_dir": "$HOME/gdn/bin",
  "cni_config_dir": "$HOME/gdn/garden-cni/config/",
  "bind_mount_dir": "$HOME/gdn/garden-cni/container-netns",
  "overlay_network" :"10.0.2.0/24",
  "state_file" : "$HOME/gdn/garden-cni/external-networker-state.json",
  "start_port" : 1000,
  "total_ports" : 2000,
  "iptables_lock_file" :"$HOME/gdn/garden-cni/iptables.lock",
  "instance_address" : "1.2.3.4",
  "iptables_asg_logging" : false
}
EOF

cat > $HOME/gdn/garden-cni/config/bridge.conf <<EOF
{
  "name": "mynet",
  "type": "bridge",
  "bridge": "mynet0",
  "isDefaultGateway": true,
  "forceAddress": false,
  "ipMasq": true,
  "hairpinMode": true,
  "ipam": {
    "type": "host-local",
    "subnet": "10.10.0.0/16"
  }
}
EOF

cat > $HOME/gdn/config/grootfs.conf <<EOF
store: /var/lib/grootfs/btrfs
driver: btrfs
log_level: info

create:
  json: true
EOF

sudo $HOME/gdn/bin/grootfs \
  --config $HOME/gdn/config/grootfs.conf \
  init-store \
  --uid-mapping "0:$(id -u):1" \
  --uid-mapping "1:$(grep $(whoami) /etc/subuid | awk -F: '{print $2":"$3}')" \
  --gid-mapping "0:$(id -g):1" \
  --gid-mapping "1:$(grep $(whoami) /etc/subgid | awk -F: '{print $2":"$3}')"
```

### Step 3: Run the gdn setup command

```
id=$(id -u) gid=$(id -g)
sudo $HOME/gdn/bin/gdn setup \
--rootless-uid "$id" \
--rootless-gid "$gid"
```

### Step 4: Run the gdn server

```
PATH=$HOME/gdn/assets/linux/bin:$HOME/gdn/assets/linux/sbin:$PATH \
$HOME/gdn/bin/gdn server \
  --assets-dir=$HOME/gdn/assets \
  --depot=/var/run/user/$(id -u)/gdn/depot \
  --bind-ip=0.0.0.0 \
  --bind-port=7777 \
  --uid-map-start=$(grep $(whoami) /etc/subuid | awk -F: '{print $2}') \
  --uid-map-length=$(grep $(whoami) /etc/subuid | awk -F: '{print $3}') \
  --gid-map-start=$(grep $(whoami) /etc/subgid | awk -F: '{print $2}') \
  --gid-map-length=$(grep $(whoami) /etc/subgid | awk -F: '{print $3}') \
  --image-plugin=$HOME/gdn/bin/grootfs \
  --image-plugin-extra-arg=--config \
  --image-plugin-extra-arg=$HOME/gdn/config/grootfs.conf \
  --network-plugin=$HOME/gdn/bin/garden-external-networker \
  --network-plugin-extra-arg=--configFile=$HOME/gdn/config/garden-cni-config.json \
  --skip-setup
```

## Using gdn

Now that the `gdn` server is up and running, we are in a position to start
creating and running rootless containers. Container operations can be performed
against the gdn server using the `gaol` CLI, available from
[here](https://github.com/contraband/gaol/releases).

If running `gaol` from the ubuntu machine, creating a container is as easy as:

```
$ gaol create -n cake -r docker:///alpine
```

If running `gaol` from elsewhere, the `-t` flag can be passed to target the
server:

```
$ gaol -t <server ip>:<server listen port> create -n cake -r docker:///alpine
```

Processes can be run in containers by using `gaol run`, for example:

```
$ gaol run cake -a -c "echo Hello Rootless :D"
```

Containers can be destroyed as follows:

```
$ gaol destroy cake
```

## Internet access

Outbound internet access from the containers can be enabled by setting
`net-out` rules:

```
$ gaol net-out cake --ip-start=0.0.0.0 --ip-end=255.255.255.255 --port-start=0 --port-end=65535 --protocol=tcp
```

It is then possible to run processes requiring internet access:

```
$ gaol run cake -a -c 'apk update'
```

Substitute the protocol/IPs/ports as appropriate.

## Known Limitations

Use of `setgroups` does not work inside a rootless container, and as such
operations that attempt to call `setgroups` will fail with a permission error.
See the [user namespaces manpage](http://man7.org/linux/man-pages/man7/user_namespaces.7.html)
for further details.
