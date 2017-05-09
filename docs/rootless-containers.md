# (Experimental) Rootless containers in Garden

With the latest releases of Garden it is now possible to create and run processes
in containers without requiring root privileges! This document details the various
components that enable this to work, as well as providing a step-by-step guide for installing
and configuring the latest Garden dev builds to run as a non-root user.

**HUGE DISCLAIMER**: Garden's support for rootless containers is still very much
a work-in-progress at the moment, and as such is subject to a number of known
limitations (see the end of this doc for details).

**Special Thanks**: A huge thanks to the [OCI community](https://www.opencontainers.org/) (and
especially [Aleksa Sarai](https://github.com/cyphar)) for their hard work and support
in making rootless containers a reality.

# Getting Started

The following documents the process of installing, configuring and running Garden
as a non-root user on an Ubuntu Xenial machine. For this document, we will assume commands
are run as the ubuntu user, you should be free to use any non-root user (subject to the prerequisites
below).

If you run into any issues along the way, feel free to chat to us on the
`#garden` channel of the [Cloud Foundry Slack](http://slack.cloudfoundry.org/).

## Prerequisites

* An Ubuntu Xenial machine (with kernel version 4.4+)
* An entry for the non-root user in the `/etc/sub{u,g}id` files
* Write permissions for the non-root user on a directory at `/var/run/user/$(id -u)`
* sudo permissions (required only for initial setup)

All prerequisites are met by default for the ubuntu user.

## Step 1: Download binaries and set permissions

A quick note on versions - the following copy-and-pastable snippts will download
the latest, bleeding-edge version of gdn. As such, it is not guaranteed to be stable.
If you would prefer to use the stable versions, they can be downloaded from the
following locations:

* [gdn](https://github.com/cloudfoundry/garden-runc-release/releases/latest)
* [grootfs](https://github.com/cloudfoundry/grootfs/releases/latest)
* [cni](https://github.com/containernetworking/cni/releases/latest)
* [garden-external-networker](https://s3.amazonaws.com/garden-external-networker/garden-external-networker) *direct download link, stable binaries not available at this time

```
sudo apt-get update -y -qq && sudo apt-get install -y -qq jq

GDN_DOWNLOAD_LINK=https://s3.amazonaws.com/gdn-linux-release/gdn
GROOTFS_DOWNLOAD_LINK=$(curl -s https://api.github.com/repos/cloudfoundry/grootfs/releases/latest | jq -r ".assets[] | select(.name | test(\"grootfs\")) | .browser_download_url")
CNI_DOWNLOAD_LINK=$(curl -s https://api.github.com/repos/containernetworking/cni/releases/latest | jq -r ".assets[] | select(.name | test(\"cni-amd64.*tgz$\")) | .browser_download_url")
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

## Step 2: Configure the image and network plugins

```
curl https://raw.githubusercontent.com/cloudfoundry/grootfs/master/hack/quick-setup | sudo bash
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
  uid_mappings:
  - "0:$(id -u):1"
  - "1:$(grep $(whoami) /etc/subuid | awk -F: '{print $2":"$3}')"
  gid_mappings:
  - "0:$(id -g):1"
  - "1:$(grep $(whoami) /etc/subgid | awk -F: '{print $2":"$3}')"
EOF
```

## Step 3: Running the gdn server

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
  --runc-root=/var/run/user/$(id -u)/runc \
  --skip-setup
```

## Using gdn

Container operations can be performed against the gdn server using the `gaol` CLI, available from [here](https://github.com/contraband/gaol/releases).

If running `gaol` from the ubuntu machine, creating a container is as easy as:

```
$ gaol create -n cake -r docker:///debian
```

If running `gaol` from elsewhere, the `-t` flag can be passed to target gdn:

```
$ gaol -t <gdn_machine_ip>:7777 create -n cake -r docker:///debian
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

Outbound internet access from the containers can be enabled by setting `net-out` rules:

```
$ gaol net-out cake --ip-start=0.0.0.0 --ip-end=255.255.255.255 --port-start=0 --port-end=65535 --protocol=tcp
```

It is then possible to run processes requiring internet access:

```
$ gaol run cake -a -c 'apt-get update'
```

Substitute the protocol/ips/ports as appropriate. Note that `gaol` CLI only supports `tcp` and `udp` protocols, so `ping` (`icmp`) will
never work. Garden itself supports all protocols.

## Known Limitations

* You cannot switch user (`su`) inside a container.
* There is currently no support for resource limiting.
* Not all images are guaranteed to work (e.g. `docker:///ubuntu` will currently error). The following are known to be ok:
  * `docker:///debian`
  * `docker:///busybox`
  * `docker:///alpine`
