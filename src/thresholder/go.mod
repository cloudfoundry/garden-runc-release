module thresholder

go 1.24.0

toolchain go1.24.2

require (
	code.cloudfoundry.org/grootfs v0.30.0
	github.com/BurntSushi/toml v1.5.0
	github.com/onsi/ginkgo/v2 v2.25.2
	github.com/onsi/gomega v1.38.2
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20250902232947-5eec23c92c42 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	go.uber.org/automaxprocs v1.6.0 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/net v0.43.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
	golang.org/x/text v0.28.0 // indirect
	golang.org/x/tools v0.36.0 // indirect
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
