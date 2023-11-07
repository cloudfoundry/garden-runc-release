module thresholder

go 1.18

require (
	code.cloudfoundry.org/grootfs v0.30.0
	github.com/onsi/ginkgo/v2 v2.13.0
	github.com/onsi/gomega v1.29.0
	gopkg.in/yaml.v2 v2.4.0
)

require (
	github.com/go-logr/logr v1.3.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20230315185526-52ccab3ef572 // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/pprof v0.0.0-20231101202521-4ca4178f5c7a // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	golang.org/x/net v0.17.0 // indirect
	golang.org/x/sys v0.14.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

replace (
	code.cloudfoundry.org/grootfs => ../grootfs
	code.cloudfoundry.org/idmapper => ../idmapper
)
