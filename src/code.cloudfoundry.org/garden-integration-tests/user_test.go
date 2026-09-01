package garden_integration_tests_test

import (
	"runtime"
	"strconv"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("users", func() {
	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
	})
	It("has a sufficiently large UID/GID range", func() {
		stdout := runForStdout(container, garden.ProcessSpec{
			User: "1000000000:1000000000",
			Path: "id",
		})
		Expect(stdout).To(gbytes.Say("uid=1000000000 gid=1000000000\n"))
	})

	Context("when creating users", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		It("creates a user with a large uid and gid", func() {
			uid := 50000
			gid := 50000

			exitCode, _, _ := runProcess(container, garden.ProcessSpec{
				User: "root",
				Path: "addgroup",
				Args: []string{"-g", strconv.Itoa(gid), "bob"},
			})
			Expect(exitCode).To(Equal(0))

			exitCode, _, _ = runProcess(container, garden.ProcessSpec{
				User: "root",
				Path: "adduser",
				Args: []string{"-u", strconv.Itoa(uid), "-G", "bob", "-D", "bob"},
			})
			Expect(exitCode).To(Equal(0))

			exitCode, _, _ = runProcess(container, garden.ProcessSpec{
				User: "bob",
				Path: "echo",
				Args: []string{"Hello Baldrick"},
			})
			Expect(exitCode).To(Equal(0))
		})
	})

	Context("when rootfs defines user/groups", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		It("ignores inherited groups from gdn but includes supplementary groups", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "alice",
				Path: "cat",
				Args: []string{"/proc/self/status"},
			})

			Expect(stdout).To(gbytes.Say(`Groups:(\s)*1010(\s)*1011(\s)*\n`))
		})
	})

	Context("when rootfs does not have an /etc/passwd", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		It("can still run as root", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "echo",
				Args: []string{"hello"},
			})

			Expect(stdout).To(gbytes.Say(`hello`))
		})

		It("fails when run as non-root", func() {
			_, err := container.Run(
				garden.ProcessSpec{
					User: "invalid-user",
					Path: "/hello",
				},
				garden.ProcessIO{},
			)
			Expect(err).To(MatchError(ContainSubstring("unable to find user invalid-user: no matching entries in passwd file")))
		})
	})
})
