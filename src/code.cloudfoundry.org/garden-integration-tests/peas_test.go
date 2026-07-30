package garden_integration_tests_test

import (
	"fmt"
	"io"
	"runtime"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

const (
	mb = 1024 * 1024
	gb = mb * 1024
)

var _ = Describe("Partially shared containers (peas)", func() {
	var (
		peaImage garden.ImageRef
		noImage  garden.ImageRef
	)

	BeforeEach(func() {
		skipIfShed()
		peaImage = garden.ImageRef{URI: gardenRootfs}
		noImage = garden.ImageRef{}
	})

	Describe("sharing of namespaces", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		It("runs a process that shares all of the namespaces besides the mount one", func() {
			sandboxContainerMntNs := getNS("mnt", container, noImage)
			peaContainerMntNs := getNS("mnt", container, peaImage)
			Expect(sandboxContainerMntNs).NotTo(Equal(peaContainerMntNs))

			for _, ns := range []string{"net", "ipc", "pid", "user", "uts"} {
				sandboxContainerNs := getNS(ns, container, noImage)
				peaContainerNs := getNS(ns, container, peaImage)
				Expect(sandboxContainerNs).To(Equal(peaContainerNs))
			}
		})

		Context("when the sandbox container is privileged", func() {
			BeforeEach(func() {
				setPrivileged()
			})

			It("runs a process that shares all of the namespaces besides the mount one", func() {
				sandboxContainerMntNs := getNS("mnt", container, noImage)
				peaContainerMntNs := getNS("mnt", container, peaImage)
				Expect(sandboxContainerMntNs).NotTo(Equal(peaContainerMntNs))

				for _, ns := range []string{"net", "ipc", "pid", "user", "uts"} {
					sandboxContainerNs := getNS(ns, container, noImage)
					peaContainerNs := getNS(ns, container, peaImage)
					Expect(sandboxContainerNs).To(Equal(peaContainerNs))
				}
			})
		})
	})

	It("runs a process in its own rootfs", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		stdout := runForStdout(container, garden.ProcessSpec{
			Path:  "busybox",
			Image: peaImage,
		})
		Expect(stdout).To(gbytes.Say(`BusyBox v`))
	})

	Describe("pea process user and group", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		It("runs the process as uid and gid 0 by default", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				Path:  "sh",
				Args:  []string{"-c", "echo -n $(id -u):$(id -g)"},
				Image: peaImage,
			})
			Expect(stdout).To(gbytes.Say("0:0"))
		})

		Context("when a uid:gid is provided", func() {
			It("runs the process as the specified uid and gid", func() {
				userGUIDs := "1001:1002"
				stdout := runForStdout(container, garden.ProcessSpec{
					Path:  "sh",
					Args:  []string{"-c", "echo -n $(id -u):$(id -g)"},
					User:  userGUIDs,
					Image: peaImage,
				})
				Expect(stdout).To(gbytes.Say(userGUIDs))
			})
		})

		Context("when a username is provided", func() {
			It("does not return an error", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User:  "operator",
					Path:  "sh",
					Args:  []string{"-c", "echo -n $(id -u):$(id -g); whoami"},
					Image: peaImage,
				})
				Expect(stdout).To(gbytes.Say("37:37operator"))
			})

			Context("but /etc/passwd is empty", func() {
				BeforeEach(func() {
					peaImage = garden.ImageRef{URI: "docker:///cloudfoundry/garden-rootfs"}
				})

				It("can run when the user is root", func() {
					stdout := runForStdout(container, garden.ProcessSpec{
						User:  "root",
						Path:  "echo",
						Args:  []string{"hello"},
						Image: peaImage,
					})
					Expect(stdout).To(gbytes.Say("hello"))
				})

				It("cannot run when the user is non-root", func() {
					_, err := container.Run(
						garden.ProcessSpec{
							User:  "invalid-user",
							Path:  "echo",
							Args:  []string{"hello"},
							Image: peaImage,
						},
						garden.ProcessIO{},
					)
					Expect(err).To(MatchError(ContainSubstring("unable to find user invalid-user: no matching entries in passwd file")))
				})
			})
		})
	})

	Describe("pea process Wait and IO", func() {
		It("returns the process exit code", func() {
			var spec garden.ProcessSpec
			if runtime.GOOS == "windows" {
				spec = garden.ProcessSpec{
					Path:  "cmd.exe",
					Args:  []string{"/c", "exit /B 123"},
					Image: peaImage,
				}
			} else {
				spec = garden.ProcessSpec{
					Path:  "sh",
					Args:  []string{"-c", "exit 123"},
					Image: peaImage,
				}
			}
			processExitCode, _, _ := runProcess(container, spec)

			Expect(processExitCode).To(Equal(123))
		})

		It("streams stdout and stderr back to the client", func() {
			var spec garden.ProcessSpec
			if runtime.GOOS == "windows" {
				spec = garden.ProcessSpec{
					Path:  "cmd.exe",
					Args:  []string{"/c", `echo stdout & echo stderr 1>&2`},
					Image: peaImage,
				}
				processExitCode, stdout, stderr := runProcess(container, spec)

				Expect(processExitCode).To(Equal(0))
				Expect(stdout).To(gbytes.Say("stdout"))
				Expect(stderr).To(gbytes.Say("stderr"))
			} else {
				spec = garden.ProcessSpec{
					Path:  "sh",
					Args:  []string{"-c", "echo stdout && echo stderr >&2"},
					Image: peaImage,
				}
				processExitCode, stdout, stderr := runProcess(container, spec)

				Expect(processExitCode).To(Equal(0))
				Expect(stdout).To(gbytes.Say("stdout\n"))
				Expect(stderr).To(gbytes.Say("stderr\n"))
			}
		})
	})

	It("bind mounts the same /etc/hosts file as the container", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		originalContentsInContainer := readFileInContainer(container, "/etc/hosts", noImage)
		originalContentsInPea := readFileInContainer(container, "/etc/hosts", peaImage)
		Expect(originalContentsInContainer).To(Equal(originalContentsInPea))

		appendFileInContainer(container, "/etc/hosts", "foobar", peaImage)
		contentsInPea := readFileInContainer(container, "/etc/hosts", peaImage)
		Expect(originalContentsInPea).NotTo(Equal(contentsInPea))

		contentsInContainer := readFileInContainer(container, "/etc/hosts", noImage)
		Expect(contentsInPea).To(Equal(contentsInContainer))
	})

	It("bind mounts the same /etc/resolv.conf file as the container", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		originalContentsInContainer := readFileInContainer(container, "/etc/resolv.conf", noImage)
		originalContentsInPea := readFileInContainer(container, "/etc/resolv.conf", peaImage)
		Expect(originalContentsInContainer).To(Equal(originalContentsInPea))

		appendFileInContainer(container, "/etc/resolv.conf", "foobar", peaImage)
		contentsInPea := readFileInContainer(container, "/etc/resolv.conf", peaImage)
		Expect(originalContentsInPea).NotTo(Equal(contentsInPea))

		contentsInContainer := readFileInContainer(container, "/etc/resolv.conf", noImage)
		Expect(contentsInPea).To(Equal(contentsInContainer))
	})

	Context("when no working directory is specified", func() {
		It("defaults to /", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			stdout := runForStdout(container, garden.ProcessSpec{
				Path:  "pwd",
				Image: peaImage,
			})
			Expect(stdout).To(gbytes.Say("/\n"))
		})
	})

	Describe("signalling", func() {
		It("sends a TERM signal to the process if requested", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", `
				trap 'echo termed; exit 42' SIGTERM

				while true; do
					echo waiting
					/bin/sleep 1
				done
			`},
				Image: peaImage,
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(GinkgoWriter, stdout),
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(stdout).Should(gbytes.Say("waiting"))
			Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
			Eventually(stdout, "2s").Should(gbytes.Say("termed"))
			Expect(process.Wait()).To(Equal(42))
		})
	})

	Describe("Limits", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			limits = garden.Limits{
				Bandwidth: garden.BandwidthLimits{RateInBytesPerSecond: mb, BurstRateInBytesPerSecond: mb},
				CPU:       garden.CPULimits{LimitInShares: 1024},
				Disk:      garden.DiskLimits{ByteHard: gb},
				Memory:    garden.MemoryLimits{LimitInBytes: 64 * mb},
				Pid:       garden.PidLimits{Max: 50},
			}
		})

		Context("when OverrideContainerLimits is not specified on the pea", func() {
			It("shares that limit with the container", func() {
				exitCode, _, _ := runProcess(container,
					garden.ProcessSpec{
						Path: "dd",
						Args: []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=5"},
					})
				Expect(exitCode).To(Equal(0))

				exitCode, _, _ = runProcess(container,
					garden.ProcessSpec{
						Path:  "dd",
						Args:  []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=60"},
						Image: peaImage,
					})
				Expect(exitCode).NotTo(Equal(0))
			})
		})

		Context("when an empty OverrideContainerLimits is specified on the pea", func() {
			It("does not share memory limit with the container", func() {
				exitCode, _, _ := runProcess(container,
					garden.ProcessSpec{
						Path:                    "dd",
						Args:                    []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=65"},
						Image:                   peaImage,
						OverrideContainerLimits: &garden.ProcessLimits{},
					})
				Expect(exitCode).To(Equal(0))
			})

			It("is not limited in how many processes it can create", func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					Path: "/bin/sh",
					Args: []string{"-c", `
					for i in $(seq 15); do (/bin/sleep 360 &); done;
					until [ $(ps aux | wc -l) -gt 15 ]; do /bin/sleep .5; done
					killall sleep
					`},
					Image:                   peaImage,
					OverrideContainerLimits: &garden.ProcessLimits{},
				})
				Expect(exitCode).To(Equal(0))
			})
		})

		Context("when a memory limit is specified on the pea", func() {
			It("kills processes that exceed that limit", func() {
				cmd := "dd if=/dev/urandom of=/dev/shm/too-big bs=1M count=33 && echo hello"
				// Temporarily checking the memory limit of the process as part of #161555465
				exitCode, stdout, stderr := runProcess(container,
					garden.ProcessSpec{
						Path:  "sh",
						Args:  []string{"-c", cmd},
						Image: peaImage,
						OverrideContainerLimits: &garden.ProcessLimits{
							Memory: garden.MemoryLimits{LimitInBytes: 32 * mb},
						},
					})
				Expect(string(stdout.Contents())).NotTo(ContainSubstring("hello\n"))
				Expect(string(stderr.Contents())).To(ContainSubstring("Killed\n"))
				Expect(exitCode).NotTo(Equal(0))
			})
		})
	})

	Context("when the sandbox is destroyed", func() {
		It("kills all associated peas", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			process, err := container.Run(garden.ProcessSpec{
				Path:  "/bin/sleep",
				Args:  []string{"10000d"},
				Image: peaImage,
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())
			Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
			exitCode, err := process.Wait()
			Expect(err).ToNot(HaveOccurred())
			Expect(exitCode).To(Equal(137)) // 128+9
		})
	})

	Describe("Metrics", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			limits = garden.Limits{Memory: garden.MemoryLimits{
				LimitInBytes: 64 * mb,
			}}
		})

		Context("when there is no memory limit on the pea", func() {
			It("should return bulk metrics", func() {
				skipIfWoot("Groot does not support metrics yet")
				buffer := gbytes.NewBuffer()

				proc, err := container.Run(
					garden.ProcessSpec{
						Path:  "sh",
						Args:  []string{"-c", `while true; do ls -la; done`},
						Image: peaImage,
					},
					garden.ProcessIO{
						Stdout: buffer,
						Stderr: GinkgoWriter,
					},
				)
				Expect(err).NotTo(HaveOccurred())
				Eventually(buffer).Should(gbytes.Say("total"))

				handle := proc.ID()
				Eventually(func() (uint64, error) {
					metrics, err := gardenClient.BulkMetrics([]string{handle})
					if err != nil {
						return 0, err
					}
					if metrics[handle].Err != nil {
						return 0, metrics[handle].Err
					}
					return metrics[handle].Metrics.MemoryStat.TotalUsageTowardLimit, nil
				}).ShouldNot(BeZero())
			})
		})
	})

	Context("when the process executable doesn't exist", func() {
		It("returns an error from Run", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			_, err := container.Run(
				garden.ProcessSpec{
					Path:  "does-not-exist",
					Args:  []string{},
					Image: peaImage,
				},
				garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				},
			)
			Expect(err).To(MatchError(ContainSubstring("executable file not found in $PATH")))
		})
	})
})

func getNS(nsName string, container garden.Container, image garden.ImageRef) string {
	processSpec := garden.ProcessSpec{
		Path:  "readlink",
		Args:  []string{fmt.Sprintf("/proc/self/ns/%s", nsName)},
		Image: image,
	}

	exitCode, namespaceInode, _ := runProcess(container, processSpec)
	Expect(exitCode).To(Equal(0))

	return string(namespaceInode.Contents())
}

func readFileInContainer(container garden.Container, filePath string, image garden.ImageRef) string {
	exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
		Path:  "cat",
		Args:  []string{filePath},
		Image: image,
	})
	Expect(exitCode).To(Equal(0))

	return string(stdout.Contents())
}

func appendFileInContainer(container garden.Container, filePath, content string, image garden.ImageRef) {
	exitCode, _, _ := runProcess(container, garden.ProcessSpec{
		Path:  "sh",
		Args:  []string{"-c", fmt.Sprintf("echo %s >> %s", content, filePath)},
		Image: image,
	})
	Expect(exitCode).To(Equal(0))
}
