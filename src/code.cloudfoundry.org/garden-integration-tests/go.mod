module code.cloudfoundry.org/garden-integration-tests

go 1.26.3

replace (
	code.cloudfoundry.org/garden => ../garden
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/guardian => ../guardian
	code.cloudfoundry.org/idmapper => ../idmapper
)

require (
	code.cloudfoundry.org/archiver v0.81.0
	code.cloudfoundry.org/garden v0.0.0-20260729020900-a0441c36f7b5
	code.cloudfoundry.org/guardian v0.0.0-20260722021324-599e660ac834
	github.com/cloudfoundry/gosigar v1.3.125
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d
	github.com/onsi/ginkgo/v2 v2.32.0
	github.com/onsi/gomega v1.42.1
	github.com/wavefronthq/wavefront-sdk-go v0.15.0
)

require (
	code.cloudfoundry.org/commandrunner v0.72.0 // indirect
	code.cloudfoundry.org/lager/v3 v3.80.0 // indirect
	github.com/Masterminds/semver/v3 v3.5.0 // indirect
	github.com/bmizerany/pat v0.0.0-20210406213842-e4b6760bdd6f // indirect
	github.com/caio/go-tdigest/v4 v4.1.0 // indirect
	github.com/cloudfoundry/dropsonde v1.1.0 // indirect
	github.com/cloudfoundry/sonde-go v0.0.0-20260720065356-6728909ed72b // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/cyphar/filepath-securejoin v0.7.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/godbus/dbus/v5 v5.2.2 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20260709232956-b9395ee17fa0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/moby/sys/mountinfo v0.7.2 // indirect
	github.com/moby/sys/reexec v0.1.0 // indirect
	github.com/moby/sys/userns v0.1.0 // indirect
	github.com/nxadm/tail v1.4.11 // indirect
	github.com/opencontainers/cgroups v0.0.8 // indirect
	github.com/opencontainers/runtime-spec v1.3.0 // indirect
	github.com/openzipkin/zipkin-go v0.4.3 // indirect
	github.com/sirupsen/logrus v1.9.4 // indirect
	github.com/tedsuo/rata v1.0.0 // indirect
	github.com/vishvananda/netlink v1.3.1 // indirect
	github.com/vishvananda/netns v0.0.5 // indirect
	go.yaml.in/yaml/v3 v3.0.5 // indirect
	golang.org/x/mod v0.38.0 // indirect
	golang.org/x/net v0.57.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.40.0 // indirect
	golang.org/x/tools v0.48.0 // indirect
	google.golang.org/protobuf v1.36.12-0.20260120151049-f2248ac996af // indirect
)
