module thresholder

go 1.23.0

toolchain go1.23.6

require (
	code.cloudfoundry.org/grootfs v0.30.0
	github.com/BurntSushi/toml v1.5.0
	github.com/onsi/ginkgo/v2 v2.23.3
	github.com/onsi/gomega v1.36.3
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-task/slim-sprig/v3 v3.0.0 // indirect
	github.com/google/go-cmp v0.7.0 // indirect
	github.com/google/pprof v0.0.0-20250317173921-a4b03ec1a45e // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.38.0 // indirect
	golang.org/x/sys v0.31.0 // indirect
	golang.org/x/text v0.23.0 // indirect
	golang.org/x/tools v0.31.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
