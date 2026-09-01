package garden_integration_tests_test

import (
	"fmt"
	"os"
	"runtime"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Devices", func() {

	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("skip for windows")
		}
	})

	DescribeTable("Devices",
		func(device string, major, minor int) {
			stdout := runForStdout(container, garden.ProcessSpec{
				Path: "ls",
				Args: []string{"-l", device},
			})

			Expect(stdout).To(gbytes.Say(fmt.Sprintf(`%d,\s*%d`, major, minor)))
		},

		Entry("should have the TTY device", "/dev/tty", 5, 0),
		Entry("should have the random device", "/dev/random", 1, 8),
		Entry("should have the urandom device", "/dev/urandom", 1, 9),
		Entry("should have the null device", "/dev/null", 1, 3),
		Entry("should have the zero device", "/dev/zero", 1, 5),
		Entry("should have the full device", "/dev/full", 1, 7),
		Entry("should have the /dev/pts/ptmx device", "/dev/pts/ptmx", 5, 2),
	)

	Context("in a privileged container", func() {
		BeforeEach(func() {
			privilegedContainer = true
		})

		It("should have the fuse device", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				Path: "ls",
				Args: []string{"-l", "/dev/fuse"},
			})

			Expect(stdout).To(gbytes.Say(`10,\s*229`))
		})

		It("allows permitting all devices", func() {
			if os.Getenv("NESTED") != "true" {
				Skip("Only supported on nested environments")
			}

			stdout := runForStdout(container, garden.ProcessSpec{
				Path: "sh",
				User: "root",
				Args: []string{"-c", `
					devices_mount_info="$( cat /proc/self/cgroup | grep devices )"
					if [ -z "$devices_mount_info" ]; then
						# cgroups not set up; must not be in a container
						return
					fi
					devices_subsytems=$(echo $devices_mount_info | cut -d: -f2)
					devices_subdir=$(echo $devices_mount_info | cut -d: -f3)

					if [ "$devices_subdir" = "/" ]; then
						# we're in the root devices cgroup; must not be in a container
						return
					fi

					if [ ! -e /tmp/devices-cgroup ]; then
						# mount our container's devices subsystem somewhere
						mkdir /tmp/devices-cgroup
						mount -t cgroup -o $devices_subsytems none /tmp/devices-cgroup
					fi

					# permit our cgroup to do everything with all devices
					echo a > /tmp/devices-cgroup${devices_subdir}/devices.allow
					cat  /tmp/devices-cgroup${devices_subdir}/devices.list
				`},
			})
			Expect(stdout).To(gbytes.Say("a \\*:\\* rwm"))
		})

	})

	DescribeTable("Process",
		func(device, fd string) {
			stdout := runForStdout(container, garden.ProcessSpec{
				Path: "ls",
				Args: []string{"-l", device},
			})

			Expect(stdout).To(gbytes.Say(fmt.Sprintf("%s -> %s", device, fd)))
		},
		Entry("should have /dev/fd", "/dev/fd", "/proc/self/fd"),
		Entry("should have /dev/stdin", "/dev/stdin", "/proc/self/fd/0"),
		Entry("should have /dev/stdout", "/dev/stdout", "/proc/self/fd/1"),
		Entry("should have /dev/stderr", "/dev/stderr", "/proc/self/fd/2"),
	)

	It("should have devpts mounted", func() {
		stdout := runForStdout(container, garden.ProcessSpec{
			User: "root",
			Path: "cat",
			Args: []string{"/proc/mounts"},
		})

		Expect(stdout).To(gbytes.Say(`devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=\d+,mode=620,ptmxmode=666`))
	})
})
