module thresholder

go 1.12

require (
	code.cloudfoundry.org/grootfs v0.0.0-00010101000000-000000000000
	github.com/onsi/ginkgo v1.8.0
	github.com/onsi/gomega v1.5.0
	gopkg.in/yaml.v2 v2.2.2
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
