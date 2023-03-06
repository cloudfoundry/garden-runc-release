module thresholder

go 1.18

require (
	code.cloudfoundry.org/grootfs v0.30.0
	github.com/onsi/ginkgo/v2 v2.8.1
	github.com/onsi/gomega v1.27.1
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.8.0 // indirect
	golang.org/x/sys v0.6.0 // indirect
	golang.org/x/text v0.8.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
