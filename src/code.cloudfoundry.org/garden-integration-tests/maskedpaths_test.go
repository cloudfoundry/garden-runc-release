package garden_integration_tests_test

import (
	"fmt"
	"runtime"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("MaskedPaths", func() {
	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
	})
	Context("when the container is unprivileged", func() {
		It("masks certain files in /proc with a null character device", func() {
			files := []string{
				"/proc/kcore",
				"/proc/sched_debug",
				"/proc/timer_list",
				"/proc/keys",
			}

			for _, file := range files {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					Path: "test",
					Args: []string{"-f", file},
				})
				// some Linux distros don't have all the /proc/* files we are looking for
				// so we test for the existence of the file before stat'ing it.
				if exitCode == 0 {
					stdout := runForStdout(container, garden.ProcessSpec{
						Path: "stat",
						Args: []string{file},
					})
					Expect(stdout).To(gbytes.Say("character special file"))
					Expect(stdout).To(gbytes.Say("Device type: 1,3"))
				}
			}

		})

		It("masks certain /proc/timer_stats with a null character device if exists", func() {
			exitCode, stdout, stderr := runProcess(container, garden.ProcessSpec{
				Path: "stat",
				Args: []string{"/proc/timer_stats"},
			})

			if exitCode == 0 {
				Expect(stdout).To(gbytes.Say("character special file"))
				Expect(stdout).To(gbytes.Say("Device type: 1,3"))
			} else {
				// /proc/timer_stats is not available on later (4.11+) kernel versions
				Expect(stderr).To(gbytes.Say("No such file or directory"))
			}
		})

		// None of our stemells that we test on have /proc/latency_stats, so runc
		// will not normally have to mask it.
		// Still, this has use as a regression test if run against a deployment
		// with this feature enabled.
		It("does not provide access to /proc/latency_stats", func() {
			exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "if [ -e /proc/latency_stats ]; then stat /proc/latency_stats ; else echo notexist && exit 42; fi"},
			})

			if exitCode == 0 {
				Expect(stdout).To(gbytes.Say("character special file"))
				Expect(stdout).To(gbytes.Say("Device type: 1,3"))
			} else if exitCode == 42 {
				Expect(stdout).To(gbytes.Say("notexist\n"))
			} else {
				Fail(fmt.Sprintf("unexpected exit status %d", exitCode))
			}
		})

		It("masks certain dirs", func() {
			dirs := []string{
				"/proc/scsi",
				"/sys/firmware",
			}
			for _, dir := range dirs {
				stdout := runForStdout(container, garden.ProcessSpec{
					Path: "ls",
					Args: []string{"-A", dir},
				})
				Expect(stdout.Contents()).To(BeEmpty(), "directory %v is not empty", dir)
			}
		})
	})
})
