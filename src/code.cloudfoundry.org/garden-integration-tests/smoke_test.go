package garden_integration_tests_test

import (
	"runtime"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("smoke tests", func() {
	JustBeforeEach(func() {
		createUser(container, "alice")
	})
	It("can run a process inside a container", func() {
		stdout := runForStdout(container, garden.ProcessSpec{
			Path: "whoami",
			User: "alice",
		})

		if runtime.GOOS == "windows" {
			Expect(stdout).To(gbytes.Say("alice\r\n"))
		} else {
			Expect(stdout).To(gbytes.Say("alice\n"))
		}
	})
})
