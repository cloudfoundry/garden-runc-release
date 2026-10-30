module code.cloudfoundry.org/guardian

go 1.26.8

replace (
	github.com/cilium/ebpf => github.com/cilium/ebpf v0.17.3
	github.com/jessevdk/go-flags => github.com/jessevdk/go-flags v1.4.0
)

require (
	code.cloudfoundry.org/archiver v0.94.0
	code.cloudfoundry.org/clock v1.94.0
	code.cloudfoundry.org/commandrunner v0.84.0
	code.cloudfoundry.org/debugserver v0.121.0
	code.cloudfoundry.org/garden v0.10.0
	code.cloudfoundry.org/lager/v3 v3.93.0
	code.cloudfoundry.org/localip v0.95.0
	github.com/BurntSushi/toml v1.6.0
	github.com/cloudfoundry/dropsonde v1.1.0
	github.com/cloudfoundry/gosigar v1.3.128
	github.com/cloudfoundry/sonde-go v0.0.0-20260818080958-d46298cd8513
	github.com/containerd/cgroups/v3 v3.1.3
	github.com/containerd/containerd/api v1.12.0
	github.com/containerd/containerd/v2 v2.4.2
	github.com/containerd/errdefs v1.0.0
	github.com/containerd/typeurl/v2 v2.3.0
	github.com/containers/image/v5 v5.36.2
	github.com/containers/storage v1.59.1
	github.com/cyphar/filepath-securejoin v0.7.0
	github.com/docker/distribution v2.8.3+incompatible
	github.com/docker/docker v28.5.2+incompatible
	github.com/docker/go-units v0.5.0
	github.com/eapache/go-resiliency v1.7.0
	github.com/hashicorp/go-multierror v1.1.1
	github.com/jessevdk/go-flags v1.6.1
	github.com/kardianos/osext v0.0.0-20190222173326-2bc1f35cddc0
	github.com/klauspost/compress v1.20.3
	github.com/maxbrunsfeld/counterfeiter/v6 v6.8.1
	github.com/mitchellh/copystructure v1.2.0
	github.com/moby/sys/reexec v0.1.0
	github.com/moby/sys/user v0.5.0
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.33.1
	github.com/onsi/gomega v1.44.0
	github.com/opencontainers/cgroups v0.1.0
	github.com/opencontainers/go-digest v1.0.0
	github.com/opencontainers/image-spec v1.1.1
	github.com/opencontainers/runc v1.5.2
	github.com/opencontainers/runtime-spec v1.3.0
	github.com/pkg/errors v0.9.1
	github.com/sirupsen/logrus v1.10.2
	github.com/spf13/pflag v1.0.10
	github.com/tedsuo/ifrit v0.0.0-20260908181113-dd353a7daa27
	github.com/urfave/cli/v2 v2.27.7
	github.com/ventu-io/go-shortid v0.0.0-20201117134242-e59966efd125
	github.com/vishvananda/netlink v1.3.1
	github.com/vishvananda/netns v0.0.5
	golang.org/x/sys v0.49.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v2 v2.4.0
)

require (
	cyphar.com/go-pathrs v0.2.6 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/Microsoft/go-winio v0.6.4 // indirect
	github.com/Microsoft/hcsshim v0.15.0 // indirect
	github.com/aperturerobotics/protobuf-go-lite v0.19.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/checkpoint-restore/go-criu/v8 v8.5.0 // indirect
	github.com/cilium/ebpf v0.22.0 // indirect
	github.com/containerd/console v1.0.5 // indirect
	github.com/containerd/continuity v0.5.0 // indirect
	github.com/containerd/errdefs/pkg v0.3.0 // indirect
	github.com/containerd/fifo v1.1.0 // indirect
	github.com/containerd/go-runc v1.2.1 // indirect
	github.com/containerd/log v0.2.0 // indirect
	github.com/containerd/log/otel v0.1.0 // indirect
	github.com/containerd/platforms v1.0.0-rc.5 // indirect
	github.com/containerd/plugin v1.1.0 // indirect
	github.com/containerd/ttrpc v1.2.10 // indirect
	github.com/containers/libtrust v0.0.0-20230121012942-c1716e8a8d01 // indirect
	github.com/containers/ocicrypt v1.3.2 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.7 // indirect
	github.com/distribution/reference v0.6.0 // indirect
	github.com/docker/docker-credential-helpers v0.9.12 // indirect
	github.com/docker/go-connections v0.8.2 // indirect
	github.com/felixge/httpsnoop v1.1.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/godbus/dbus/v5 v5.2.4 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20261026192309-dbd0ed30aa88 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/mitchellh/reflectwalk v1.0.2 // indirect
	github.com/moby/locker v1.0.1 // indirect
	github.com/moby/sys/capability v0.5.0 // indirect
	github.com/moby/sys/mountinfo v0.7.3 // indirect
	github.com/moby/sys/signal v0.7.1 // indirect
	github.com/moby/sys/userns v0.2.1 // indirect
	github.com/mrunalp/fileutils v0.5.1 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/opencontainers/selinux v1.15.1 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/seccomp/libseccomp-golang v0.12.0 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/teris-io/shortid v0.0.0-20201117134242-e59966efd125 // indirect
	github.com/urfave/cli v1.22.17 // indirect
	github.com/xrash/smetrics v0.0.0-20261013231157-39c47d93efba // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp v0.72.0 // indirect
	go.opentelemetry.io/otel v1.47.0 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/metric v1.47.0 // indirect
	go.opentelemetry.io/otel/sdk v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/net v0.62.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	golang.org/x/tools v0.52.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20261026165753-2ffab32bce7e // indirect
	google.golang.org/grpc v1.85.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
