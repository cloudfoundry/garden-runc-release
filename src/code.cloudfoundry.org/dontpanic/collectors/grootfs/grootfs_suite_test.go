package grootfs_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGrootfs(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Grootfs Suite")
}
