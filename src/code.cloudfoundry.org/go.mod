module code.cloudfoundry.org

go 1.26.3

require (
	code.cloudfoundry.org/archiver v0.86.0
	code.cloudfoundry.org/garden v0.0.0-20260826021137-674e52f7c65b
	code.cloudfoundry.org/guardian v0.0.0-20260820194234-c2d9348cfdee
	code.cloudfoundry.org/lager/v3 v3.85.0
	github.com/BurntSushi/toml v1.6.0
	github.com/cloudfoundry/gosigar v1.3.126
	github.com/jessevdk/go-flags v1.6.1
	github.com/logrusorgru/aurora v2.0.3+incompatible
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.32.1
	github.com/onsi/gomega v1.43.0
	github.com/wavefronthq/wavefront-sdk-go v0.15.0
	golang.org/x/sys v0.47.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	code.cloudfoundry.org/commandrunner v0.73.0 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/caio/go-tdigest/v4 v4.1.0 // indirect
	github.com/cloudfoundry/dropsonde v1.1.0 // indirect
	github.com/cloudfoundry/sonde-go v0.0.0-20260818080958-d46298cd8513 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/cyphar/filepath-securejoin v0.7.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260902005441-ca85771921e4 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/reexec v0.1.0 // indirect
	github.com/moby/sys/userns v0.2.0 // indirect
	github.com/opencontainers/cgroups v0.1.0 // indirect
	github.com/opencontainers/runtime-spec v1.3.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/rogpeppe/go-internal v1.14.1 // indirect
	github.com/sirupsen/logrus v1.10.2 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/net v0.58.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	golang.org/x/tools v0.49.0 // indirect
	gonum.org/v1/gonum v0.17.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
)

replace code.cloudfoundry.org/guardian => ./guardian

replace github.com/cilium/ebpf => github.com/cilium/ebpf v0.17.3
