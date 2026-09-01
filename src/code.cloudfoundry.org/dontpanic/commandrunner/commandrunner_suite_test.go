package commandrunner_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCommandrunner(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Commandrunner Suite")
}
