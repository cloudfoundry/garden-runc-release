module thresholder

go 1.12

require (
	code.cloudfoundry.org/grootfs v0.0.0-00010101000000-000000000000
	github.com/onsi/ginkgo v1.11.0
	github.com/onsi/gomega v1.8.1
	gopkg.in/yaml.v2 v2.2.7
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
