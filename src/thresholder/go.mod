module thresholder

go 1.16

require (
	code.cloudfoundry.org/grootfs v0.30.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	gopkg.in/yaml.v2 v2.4.0
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
