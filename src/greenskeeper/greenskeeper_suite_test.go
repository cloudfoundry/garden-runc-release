package greenskeeper

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGreenskeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Greenskeeper Suite")
}
