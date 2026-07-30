package garden_integration_tests_test

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	archiver "code.cloudfoundry.org/archiver/extractor/test_helper"
	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	uuid "github.com/nu7hatch/gouuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Lifecycle", func() {
	JustBeforeEach(func() {
		createUser(container, "alice")
	})

	var (
		adminUser   string
		regularUser string
		shell       string
	)

	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			adminUser = ""
			regularUser = "alice"
			shell = "cmd.exe"
		} else {
			adminUser = "root"
			regularUser = "alice"
			shell = "sh"
		}
	})

	Context("Creating a container with limits", func() {
		BeforeEach(func() {
			limits = garden.Limits{
				Memory: garden.MemoryLimits{
					LimitInBytes: 1024 * 1024 * 128,
				},
			}
		})

		It("it applies limits if set in the container spec", func() {
			memoryLimit, err := container.CurrentMemoryLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(memoryLimit).To(Equal(limits.Memory))
		})
		When("In cgroups-v1", func() {
			BeforeEach(func() {
				if cgroups.IsCgroup2UnifiedMode() {
					Skip("for cgroups-v1 only")
				}
				limits.CPU = garden.CPULimits{LimitInShares: 50}
			})
			It("it applies limits if set in the container spec", func() {
				cpuLimit, err := container.CurrentCPULimits()
				Expect(err).ToNot(HaveOccurred())
				Expect(cpuLimit).To(Equal(limits.CPU))
			})
		})
		When("In cgroups-v2", func() {
			BeforeEach(func() {
				if !cgroups.IsCgroup2UnifiedMode() {
					Skip("for cgroups-v2 only")
				}
				limits.CPU = garden.CPULimits{Weight: 1028, LimitInShares: 0}
			})
			It("it applies limits if set in the container spec", func() {
				cpuLimit, err := container.CurrentCPULimits()
				Expect(err).ToNot(HaveOccurred())
				value := garden.CPULimits{LimitInShares: cgroups.ConvertCPUSharesToCgroupV2Value(limits.CPU.Weight)}
				Expect(cpuLimit).To(Equal(value))
			})
		})

		It("does not apply limits if not set in container spec", func() {
			diskLimit, err := container.CurrentDiskLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(diskLimit).To(Equal(garden.DiskLimits{}))

			bandwidthLimit, err := container.CurrentBandwidthLimits()
			Expect(err).ToNot(HaveOccurred())
			Expect(bandwidthLimit).To(Equal(garden.BandwidthLimits{}))
		})

		It("should be able to create and destroy containers sequentially", func() {
			skipIfWoot("Groot does not support destroy yet")
			var diskLimits garden.DiskLimits
			if runtime.GOOS == "windows" {
				diskLimits = garden.DiskLimits{
					ByteHard: 8.5 * 1024 * 1024 * 1024,
				}
			} else {
				diskLimits = garden.DiskLimits{
					ByteHard: 2 * 1024 * 1024 * 1024,
				}
			}

			container1, err := gardenClient.Create(garden.ContainerSpec{Limits: garden.Limits{Disk: diskLimits}})
			Expect(err).NotTo(HaveOccurred())
			container2, err := gardenClient.Create(garden.ContainerSpec{Limits: garden.Limits{Disk: diskLimits}})
			Expect(err).NotTo(HaveOccurred())

			Expect(gardenClient.Destroy(container1.Handle())).To(Succeed())
			Expect(gardenClient.Destroy(container2.Handle())).To(Succeed())
		})
	})

	Context("Creating a container with a duplicate handle", func() {
		It("returns a meaningful error message", func() {
			existingHandle := container.Handle()

			container, err := gardenClient.Create(garden.ContainerSpec{
				Handle: existingHandle,
			})

			Expect(container).To(BeNil())
			Expect(err).To(MatchError(fmt.Sprintf("Handle '%s' already in use", existingHandle)))
		})
	})

	checkMappings := func(mappingType string) {
		stdout := runForStdout(container, garden.ProcessSpec{
			Path: "cat",
			Args: []string{fmt.Sprintf("/proc/self/%sid_map", mappingType)},
		})

		mappingSize := `0\s+4294967294\s+1\n\s+1\s+1\s+4294967293`
		Expect(stdout).To(gbytes.Say(mappingSize))
	}

	Describe("Creating a container with uid/gid mappings", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		It("should have the proper uid mappings", func() {
			checkMappings("u")
		})

		It("should have the proper gid mappings", func() {
			checkMappings("g")
		})
	})

	It("returns garden.ContainerNotFound when destroying a container that doesn't exist", func() {
		Expect(gardenClient.Destroy("potato-sandwhich-policy")).To(MatchError(garden.ContainerNotFoundError{Handle: "potato-sandwhich-policy"}))
	})

	It("provides /dev/shm as tmpfs in the container", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		exitCode, stdout, stderr := runProcess(container, garden.ProcessSpec{
			User: "alice",
			Path: "dd",
			Args: []string{"if=/dev/urandom", "of=/dev/shm/some-data", "count=64", "bs=1k"},
		})

		fmt.Println(string(stdout.Contents()))
		fmt.Println(string(stderr.Contents()))
		Expect(exitCode).To(Equal(0))

		stdout = runForStdout(container, garden.ProcessSpec{
			User: "alice",
			Path: "cat",
			Args: []string{"/proc/mounts"},
		})

		Expect(stdout).To(gbytes.Say("tmpfs /dev/shm tmpfs rw,nosuid,nodev,noexec,relatime"))
	})

	It("gives the container a hostname based on its handle", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		stdout := runForStdout(container, garden.ProcessSpec{
			User: "alice",
			Path: "hostname",
		})

		Eventually(stdout).Should(gbytes.Say(fmt.Sprintf("%s\n", container.Handle())))
	})

	It("runs garden-init as pid 1", func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		stdout := runForStdout(container, garden.ProcessSpec{
			Path: "head",
			Args: []string{"-n1", "/proc/1/status"},
		})
		Expect(stdout).To(gbytes.Say("garden-init"))
	})

	Context("when the handle is bigger than 49 characters", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			handle = "7132-ec774112a9cd-101f8293-230e-4fa8-4138-e8244e6dcfa1"
		})

		It("should use the last 49 characters of the handle as the hostname", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "alice",
				Path: "hostname",
			})

			Eventually(stdout).Should(gbytes.Say("ec774112a9cd-101f8293-230e-4fa8-4138-e8244e6dcfa1"))
		})
	})

	Context("and sending a List request", func() {
		It("includes the created container", func() {
			Expect(getContainerHandles()).To(ContainElement(container.Handle()))
		})
	})

	Context("and sending an Info request", func() {
		It("returns the container's info", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			info, err := container.Info()
			Expect(err).ToNot(HaveOccurred())

			Expect(info.State).To(Equal("active"))
		})
	})

	Describe("attaching to a process", func() {
		Context("when the process doesn't exist", func() {
			It("returns a ProcessNotFound error", func() {
				_, err := container.Attach("idontexist", garden.ProcessIO{})
				Expect(err).To(MatchError(garden.ProcessNotFoundError{ProcessID: "idontexist"}))
			})
		})
	})

	Describe("running a process", func() {
		Context("when root is requested", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
			})
			It("runs as root inside the container", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					Path: "whoami",
					User: "root",
				})

				Expect(stdout).To(gbytes.Say("root\n"))
			})
		})

		It("streams output back and reports the exit status", func() {
			var args []string
			if runtime.GOOS == "windows" {
				args = []string{"/C", `echo %FIRST% & echo %SECOND% 1>&2 & exit /B 42`}
			} else {
				args = []string{"-c", "/bin/sleep 0.5; echo $FIRST; /bin/sleep 0.5; echo $SECOND >&2; /bin/sleep 0.5; exit 42"}
			}
			exitCode, stdout, stderr := runProcess(container, garden.ProcessSpec{
				User: regularUser,
				Path: shell,
				Args: args,
				Env:  []string{"FIRST=hello", "SECOND=goodbye"},
			})

			Expect(exitCode).To(Equal(42))
			if runtime.GOOS == "windows" {
				Expect(stdout).To(gbytes.Say("hello\\s+\r\n"))
				Expect(stderr).To(gbytes.Say("goodbye\\s+\r\n"))
			} else {
				Expect(stdout).To(gbytes.Say("hello\n"))
				Expect(stderr).To(gbytes.Say("goodbye\n"))
			}
		})

		It("can use /dev/stdin", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			stdinR, stdinW, err := os.Pipe()
			Expect(err).NotTo(HaveOccurred())
			defer stdinR.Close()

			stdout := gbytes.NewBuffer()
			pio := garden.ProcessIO{
				Stdin:  stdinR,
				Stdout: stdout,
			}
			proc, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "cat",
				Args: []string{"/dev/stdin"},
			}, pio)
			Expect(err).NotTo(HaveOccurred())

			fmt.Fprintln(stdinW, "potato")
			Eventually(stdout).Should(gbytes.Say("potato"))

			stdinW.Close()

			exitCode, err := proc.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))
		})

		It("can use /dev/stdout", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
				User: "alice",
				Path: "sh",
				Args: []string{"-c", "echo -n potato >/dev/stdout"},
			})

			Expect(exitCode).To(Equal(0))
			Expect(stdout).To(gbytes.Say("potato"))
		})

		It("can use /dev/stderr", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
				User: "alice",
				Path: "sh",
				Args: []string{"-c", "echo -n cake >/dev/stderr"},
			})

			Expect(exitCode).To(Equal(0))
			Expect(stderr).To(gbytes.Say("cake"))
		})

		Context("when multiple clients attach to the same process", func() {
			It("all clients attached should get the exit code", func() {
				var args []string
				if runtime.GOOS == "windows" {
					args = []string{"/C", `waitfor twosec /T 2 & exit /B 12`}
				} else {
					args = []string{"-c", `/bin/sleep 2; exit 12`}
				}
				process, err := container.Run(garden.ProcessSpec{
					Path: shell,
					Args: args,
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				wg := sync.WaitGroup{}
				for i := 0; i <= 5; i++ {
					wg.Add(1)
					go func() {
						defer wg.Done()
						defer GinkgoRecover()
						proc, err := container.Attach(process.ID(), garden.ProcessIO{})
						Expect(err).ToNot(HaveOccurred())
						code, err := proc.Wait()
						Expect(err).NotTo(HaveOccurred())
						Expect(code).To(Equal(12))
					}()
				}
				wg.Wait()
			})

			It("should be able to get the exitcode multiple times on the same process", func() {
				var args []string
				if runtime.GOOS == "windows" {
					args = []string{"/C", `waitfor twosec /T 2 & exit /B 12`}
				} else {
					args = []string{"-c", `/bin/sleep 2; exit 12`}
				}
				process, err := container.Run(garden.ProcessSpec{
					Path: shell,
					Args: args,
				}, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())

				for i := 0; i < 3; i++ {
					code, err := process.Wait()
					Expect(err).ToNot(HaveOccurred())
					Expect(code).To(Equal(12))
				}
			})
		})

		It("all attached clients should get stdout and stderr", func() {
			skipIfContainerdForProcesses()
			var runStdout, attachStdout, runStderr, attachStderr bytes.Buffer

			var args []string
			if runtime.GOOS == "windows" {
				args = []string{"/C", `@echo off & waitfor tensec /T 10 & for /l %x in (1, 1, 10) do (echo %x & echo %x 1>&2)`}
			} else {
				args = []string{"-c", `/bin/sleep 1; for i in $(seq 1 10); do echo $i; echo $i >&2; done`}
			}

			process, err := container.Run(garden.ProcessSpec{
				Path: shell,
				Args: args,
			}, garden.ProcessIO{
				Stdout: io.MultiWriter(&runStdout, GinkgoWriter),
				Stderr: io.MultiWriter(&runStderr, GinkgoWriter),
			})
			Expect(err).ToNot(HaveOccurred())

			attachedProcess, err := container.Attach(process.ID(), garden.ProcessIO{
				Stdout: io.MultiWriter(&attachStdout, GinkgoWriter),
				Stderr: io.MultiWriter(&attachStderr, GinkgoWriter),
			})
			Expect(err).NotTo(HaveOccurred())

			exitCode, err := process.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))

			// Looks redundant, but avoids race as we have 2 representations of the process
			exitCode, err = attachedProcess.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(0))

			Expect(runStdout.String()).To(Equal("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), "1st buffer:")
			Expect(attachStdout.String()).To(Equal("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), "2nd buffer:")

			Expect(runStderr.String()).To(Equal("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), "1st buffer:")
			Expect(attachStderr.String()).To(Equal("1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"), "2nd buffer:")
		})

		It("sends a TERM signal to the process if requested", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "sh",
				Args: []string{"-c", `
				trap 'echo termed; sleep 1; exit 42' SIGTERM

				while true; do
					echo waiting
					/bin/sleep 1
				done
			`},
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

		It("sends a TERM signal to the process run by root if requested", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{"-c", `
				trap 'echo termed; /bin/sleep 1; exit 42' SIGTERM

				while true; do
					echo waiting
					/bin/sleep 1
				done
			`},
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

		Context("even when /bin/kill does not exist", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
			})
			JustBeforeEach(func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: adminUser,
					Path: "rm",
					Args: []string{"/bin/kill"},
				})
				Expect(exitCode).To(Equal(0))
			})

			checkProcessIsGone := func(container garden.Container, argsPrefix string) {
				Consistently(func() *gbytes.Buffer {
					stdout := runForStdout(container, garden.ProcessSpec{
						User: regularUser,
						Path: "ps",
						Args: []string{"ax", "-o", "args="},
					})

					return stdout
				}).ShouldNot(gbytes.Say(argsPrefix))
			}

			It("sends a KILL signal to the process if requested", func() {
				done := make(chan interface{})
				go func() {
					stdout := gbytes.NewBuffer()
					id, err := uuid.NewV4()
					Expect(err).ToNot(HaveOccurred())
					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Path: "sh",
						Args: []string{
							"-c", fmt.Sprintf(`
							echo %s
							trap wait SIGTERM

							while true; do
							  echo waiting
								/bin/sleep 1
							done
						`, id.String()),
						},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Eventually(stdout).Should(gbytes.Say("waiting"))

					Expect(process.Signal(garden.SignalKill)).To(Succeed())
					Expect(process.Wait()).To(Equal(137))

					checkProcessIsGone(container, id.String())

					close(done)
				}()
				Eventually(done, 10.0).Should(BeClosed())
			})

			It("sends a TERMINATE signal to the process if requested", func() {
				done := make(chan interface{})
				go func() {
					id, err := uuid.NewV4()
					Expect(err).NotTo(HaveOccurred())
					stdout := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Path: "sh",
						Args: []string{"-c", fmt.Sprintf(`
							echo %s
							while true; do
							  echo waiting
								/bin/sleep 1
							done
						`, id.String())},
					}, garden.ProcessIO{
						Stdout: io.MultiWriter(GinkgoWriter, stdout),
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())
					Eventually(stdout).Should(gbytes.Say("waiting"))

					Expect(process.Signal(garden.SignalTerminate)).To(Succeed())
					Expect(process.Wait()).NotTo(BeZero())

					checkProcessIsGone(container, id.String())

					close(done)
				}()
				Eventually(done, 10.0).Should(BeClosed())
			})

			Context("when killing a process that does not use streaming", func() {
				var process garden.Process
				var buff *gbytes.Buffer

				JustBeforeEach(func() {
					var err error

					buff = gbytes.NewBuffer()
					process, err = container.Run(garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{
							"-c", "while true; do echo stillhere; /bin/sleep 1; done",
						},
					}, garden.ProcessIO{Stdout: buff})
					Expect(err).ToNot(HaveOccurred())

					Eventually(buff).Should(gbytes.Say("stillhere")) // make sure we dont kill before the process is spawned to avoid false-positives
					Expect(process.Signal(garden.SignalKill)).To(Succeed())
				})

				It("goes away", func() {
					done := make(chan interface{})
					go func() {
						Expect(process.Wait()).NotTo(Equal(0))
						Consistently(buff, "5s").ShouldNot(gbytes.Say("stillhere"))
						close(done)
					}()
					Eventually(done, 30).Should(BeClosed())
				})
			})
		})

		It("avoids a race condition when sending a kill signal", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			done := make(chan interface{})
			go func() {
				for i := 0; i < 20; i++ {
					process, err := container.Run(garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{"-c", `while true; do echo -n "x"; /bin/sleep 1; done`},
					}, garden.ProcessIO{
						Stdout: GinkgoWriter,
						Stderr: GinkgoWriter,
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Signal(garden.SignalKill)).To(Succeed())
					Expect(process.Wait()).NotTo(Equal(0))
				}

				close(done)
			}()
			Eventually(done, 480).Should(BeClosed())
		})

		It("collects the process's full output when tty is requested", func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			command := `seq -s " " 10000`
			if isContainerdForProcesses() {
				// getting process output when using containerd for processes is a bit flaky, therefore delay the process a bit so that its output can be collected
				// see https://github.com/containerd/containerd/issues/4107
				command = `seq -s " " 10000 && /bin/sleep 1`
			}
			for i := 0; i < 100; i++ {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", command},
					TTY:  new(garden.TTYSpec),
				})

				Expect(stdout).To(gbytes.Say("9999 10000"))
			}
		})

		It("collects the process's full output, even if it exits quickly after", func() {
			for i := 0; i < 100; i++ {
				stdout := gbytes.NewBuffer()

				if runtime.GOOS == "windows" {
					process, err := container.Run(garden.ProcessSpec{
						User: regularUser,
						Path: "findstr",
						Args: []string{".*"},
					}, garden.ProcessIO{
						Stdin:  bytes.NewBuffer([]byte("hi stdout\n")),
						Stderr: os.Stderr,
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					Eventually(stdout).Should(gbytes.Say("hi stdout"))
					Expect(process.Wait()).To(Equal(0))
				} else {
					process, err := container.Run(garden.ProcessSpec{
						User: regularUser,
						Path: "echo",
						Args: []string{"hi stdout"},
					}, garden.ProcessIO{
						Stdout: stdout,
					})
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(0))
					Expect(stdout).To(gbytes.Say("hi stdout"))
				}
			}
		})

		It("streams input to the process's stdin", func() {
			if runtime.GOOS == "windows" {
				stdout := gbytes.NewBuffer()
				pio := garden.ProcessIO{
					Stdin:  bytes.NewBufferString("hello\nworld\n"),
					Stdout: stdout,
				}

				process, err := container.Run(garden.ProcessSpec{
					User: regularUser,
					Path: "findstr",
					Args: []string{".*"},
				}, pio)
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("hello\nworld\n"))

				exitCode, err := process.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(Equal(0))
			} else {
				stdinR, stdinW, err := os.Pipe()
				Expect(err).NotTo(HaveOccurred())
				defer stdinR.Close()

				stdout := gbytes.NewBuffer()
				pio := garden.ProcessIO{
					Stdin:  stdinR,
					Stdout: stdout,
				}

				process, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", "cat <&0"},
				}, pio)
				Expect(err).ToNot(HaveOccurred())

				fmt.Fprintln(stdinW, "hello\nworld")
				Eventually(stdout).Should(gbytes.Say("hello\nworld"))

				stdinW.Close()

				exitCode, err := process.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(Equal(0))
			}
		})

		It("forwards the exit status even if stdin is still being written", func() {
			// this covers the case of intermediaries shuffling i/o around (e.g. wsh)
			// receiving SIGPIPE on write() due to the backing process exiting without
			// flushing stdin
			//
			// in practice it's flaky; sometimes write() finishes just before the
			// process exits, so run it ~10 times (observed it fail often in this range)
			var spec garden.ProcessSpec
			if runtime.GOOS == "windows" {
				spec = garden.ProcessSpec{
					User: regularUser,
					Path: shell,
					Args: []string{"/C", "dir"},
				}
			} else {
				spec = garden.ProcessSpec{
					User: regularUser,
					Path: "ls",
				}
			}

			for i := 0; i < 10; i++ {
				process, err := container.Run(spec, garden.ProcessIO{
					Stdin: bytes.NewBufferString(strings.Repeat("x", 1024)),
				})
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0), func() string {
					cmd := exec.Command("sh", "-c", "netstat -tna | grep 7777")
					session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
					Expect(err).NotTo(HaveOccurred())
					Eventually(session).Should(gexec.Exit())
					return string(session.Out.Contents())
				}())
			}
		})

		Context("with a tty", func() {
			It("executes the process with a raw tty with the default window size", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				stdout := gbytes.NewBuffer()
				_, err := container.Run(garden.ProcessSpec{
					User: regularUser,
					Path: shell,
					Args: []string{
						"-c",
						`
						# The mechanism that is used to set TTY size (ioctl) is
						# asynchronous. Hence, stty does not return the correct result
						# right after the process is launched.
						while true; do
							stty -a
							/bin/sleep 1
						done
					`,
					},
					TTY: new(garden.TTYSpec),
				}, garden.ProcessIO{
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout, "3s").Should(gbytes.Say("rows 24; columns 80;"))
			})

			It("executes the process with a raw tty with the given window size", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				stdout := gbytes.NewBuffer()
				_, err := container.Run(garden.ProcessSpec{
					User: regularUser,
					Path: shell,
					Args: []string{
						"-c",
						`
						# The mechanism that is used to set TTY size (ioctl) is
						# asynchronous. Hence, stty does not return the correct result
						# right after the process is launched.
						while true; do
							stty -a
							/bin/sleep 1
						done
					`,
					},
					TTY: &garden.TTYSpec{
						WindowSize: &garden.WindowSize{
							Columns: 123,
							Rows:    456,
						},
					},
				}, garden.ProcessIO{
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout, "3s").Should(gbytes.Say("rows 456; columns 123;"))
			})

			It("executes the process with a raw tty and with onlcr to preserve formatting (\r\n, not just \n)", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				stdout := gbytes.NewBuffer()
				_, err := container.Run(garden.ProcessSpec{
					Path: shell,
					Args: []string{
						"-c",
						`
						while true; do
							echo -e "new\nline"
							/bin/sleep 1
					  done
					`,
					},
					TTY: &garden.TTYSpec{},
				}, garden.ProcessIO{
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("new\r\nline"))
			})

			It("can have its terminal resized", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				skipIfContainerdForProcesses()
				stdout := gbytes.NewBuffer()

				inR, inW := io.Pipe()

				process, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{
						"-c",
						`
						trap "stty -a" SIGWINCH

						# continuously read so that the trap can keep firing
						while true; do
							echo waiting
							if read; then
								exit 0
							fi
						done
					`,
					},
					TTY: &garden.TTYSpec{
						WindowSize: &garden.WindowSize{
							Columns: 13,
							Rows:    46,
						},
					},
				}, garden.ProcessIO{
					Stdin:  inR,
					Stdout: stdout,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("waiting"))

				err = process.SetTTY(garden.TTYSpec{
					WindowSize: &garden.WindowSize{
						Columns: 123,
						Rows:    456,
					},
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout).Should(gbytes.Say("rows 456; columns 123;"))

				_, err = fmt.Fprintf(inW, "ok\n")
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(0))
			})

			It("all attached clients should get stdout and stderr", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				skipIfContainerdForProcesses()

				var runStdout, attachStdout bytes.Buffer
				stdinR, stdinW := io.Pipe()
				defer stdinW.Close()

				process, err := container.Run(garden.ProcessSpec{
					Path: shell,
					Args: []string{"-c", `
read -s

for i in $(seq 1 5); do
	echo $i
	echo $i >&2
done
					`},
					TTY: new(garden.TTYSpec),
				}, garden.ProcessIO{
					Stdin:  stdinR,
					Stdout: io.MultiWriter(&runStdout, GinkgoWriter),
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				attachedProcess, err := container.Attach(process.ID(), garden.ProcessIO{
					Stdout: io.MultiWriter(&attachStdout, GinkgoWriter),
					Stderr: GinkgoWriter,
				})
				Expect(err).NotTo(HaveOccurred())

				_, err = fmt.Fprintf(stdinW, "ok\n")
				Expect(err).ToNot(HaveOccurred())

				exitCode, err := process.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(Equal(0))

				// Looks redundant, but avoids race as we have 2 representations of the process
				exitCode, err = attachedProcess.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(Equal(0))

				expected := `(ok\r\n)?1\r\n1\r\n2\r\n2\r\n3\r\n3\r\n4\r\n4\r\n5\r\n5\r\n`
				Expect(runStdout.String()).To(MatchRegexp(expected), "run buffer:")
				Expect(attachStdout.String()).To(MatchRegexp(expected), "attach buffer:")
			})
		})

		Context("with a working directory", func() {
			It("executes with the working directory as the dir", func() {
				var spec garden.ProcessSpec
				if runtime.GOOS == "windows" {
					spec = garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{"/C", `echo %cd%`},
						Dir:  "C:\\usr",
					}
					stdout := runForStdout(container, spec)
					Eventually(stdout).Should(gbytes.Say(`C:\\usr` + "\r\n"))
				} else {
					spec = garden.ProcessSpec{
						User: regularUser,
						Path: "pwd",
						Dir:  "/usr",
					}
					stdout := runForStdout(container, spec)

					Eventually(stdout).Should(gbytes.Say("/usr\n"))
				}
			})
		})

		Context("and then sending a stop request", func() {
			It("terminates all running processes", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				stdout := gbytes.NewBuffer()

				process, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{
						"-c",
						`
					trap 'exit 42' SIGTERM

					# sync with test, and allow trap to fire when not sleeping
					while true; do
						echo waiting
						/bin/sleep 1
					done
					`,
					},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: GinkgoWriter,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout, 30).Should(gbytes.Say("waiting"))

				err = container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				Expect(process.Wait()).To(Equal(42))
			})

			It("recursively terminates all child processes", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				done := make(chan interface{})
				go func() {
					defer close(done)

					stderr := gbytes.NewBuffer()

					process, err := container.Run(garden.ProcessSpec{
						User: "alice",
						Path: "sh",
						Args: []string{
							"-c",
							`
					# don't die until child processes die
					trap wait SIGTERM

					# spawn child that exits when it receives TERM
					sh -c 'trap wait SIGTERM; /bin/sleep 100 & wait' &

					# sync with test. Use stderr to avoid buffering in the shell.
					echo waiting >&2

					# wait on children
					wait
					`,
						},
					}, garden.ProcessIO{
						Stderr: stderr,
					})

					Expect(err).ToNot(HaveOccurred())

					Eventually(stderr, 5).Should(gbytes.Say("waiting\n"))

					stoppedAt := time.Now()

					err = container.Stop(false)
					Expect(err).ToNot(HaveOccurred())

					Expect(process.Wait()).To(Equal(143)) // 143 = 128 + SIGTERM

					Expect(time.Since(stoppedAt)).To(BeNumerically("<=", 9*time.Second))
				}()
				Eventually(done, 15).Should(BeClosed())
			})

			It("changes the container's state to 'stopped'", func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				err := container.Stop(false)
				Expect(err).ToNot(HaveOccurred())

				info, err := container.Info()
				Expect(err).ToNot(HaveOccurred())

				Expect(info.State).To(Equal("stopped"))
			})

			Context("when a process does not die 10 seconds after receiving SIGTERM", func() {
				It("is forcibly killed", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					stdout := gbytes.NewBuffer()
					process, err := container.Run(garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{
							"-c",
							`
							trap "echo cannot touch this" SIGTERM

							echo waiting
							while true
							do
								/bin/sleep 1000
							done
						`,
						},
					}, garden.ProcessIO{Stdout: stdout})

					Eventually(stdout).Should(gbytes.Say("waiting"))

					Expect(err).ToNot(HaveOccurred())

					stoppedAt := time.Now()

					err = container.Stop(false)
					Expect(err).ToNot(HaveOccurred())

					exitStatus, err := process.Wait()
					Expect(err).ToNot(HaveOccurred())
					if exitStatus != 137 && exitStatus != 255 {
						Fail(fmt.Sprintf("Unexpected exitStatus: %d", exitStatus))
					}

					Expect(time.Since(stoppedAt)).To(BeNumerically(">=", 10*time.Second))
				})
			})
		})

		Context("and streaming files in", func() {
			var tarStream io.Reader
			var filesToArchive []archiver.ArchiveFile

			BeforeEach(func() {
				filesToArchive = append(filesToArchive, []archiver.ArchiveFile{
					{
						Name: "./some-temp-dir",
						Dir:  true,
					}, {
						Name: "./some-temp-dir/some-temp-file",
						Body: "some-body",
					},
				}...)
			})

			JustBeforeEach(func() {
				tmpdir, err := os.MkdirTemp("", "some-temp-dir-parent")
				Expect(err).ToNot(HaveOccurred())

				tgzPath := filepath.Join(tmpdir, "some.tgz")

				archiver.CreateTarGZArchive(tgzPath, filesToArchive)

				tgz, err := os.Open(tgzPath)
				Expect(err).ToNot(HaveOccurred())

				tarStream, err = gzip.NewReader(tgz)
				Expect(err).ToNot(HaveOccurred())
			})

			Context("when streamed files + rootfs image have xattrs on files", func() {
				BeforeEach(func() {
					if runtime.GOOS == "windows" {
						Skip("xattr testing doesn't make sense on windows")
					}
					imageRef.URI = "docker:///cloudfoundry/garden-fuse"

					var capabilities = "0100000200200000000000000000000000000000" // output from `getfattr -e hex -d -m '' /bin/ping`
					capBytes, err := hex.DecodeString(capabilities)
					Expect(err).NotTo(HaveOccurred())

					filesToArchive = append(filesToArchive, archiver.ArchiveFile{
						Name:   "./some-temp-dir/some-temp-file-with-xattrs",
						Body:   "some-body",
						Xattrs: map[string]string{"security.capability": string(capBytes)},
					})
				})

				It("preserves the xattrs for files", func() {
					if runtime.GOOS == "windows" {
						Skip("xattr testing doesn't make sense on windows")
					}

					By("Ensuring xattrs are set when streaming content into the container")
					err := container.StreamIn(garden.StreamInSpec{
						User:      "root",
						Path:      "/home/alice",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())

					exitCode, stdout, stderr := runProcess(container, garden.ProcessSpec{
						User: "root",
						Path: "/usr/bin/getfattr",
						Args: []string{"-d", "--absolute-names", "-m", "-", "-e", "hex", "/home/alice/some-temp-dir/some-temp-file-with-xattrs"},
					})
					Expect(exitCode).To(Equal(0))
					Expect(stderr).To(gbytes.Say("^$"))
					Expect(stdout).To(gbytes.Say("# file: /home/alice/some-temp-dir/some-temp-file-with-xattrs\nsecurity.capability=0x0100000200200000000000000000000000000000"))

					By("Ensuring xattrs on the rootfs image are preserved")
					exitCode, stdout, stderr = runProcess(container, garden.ProcessSpec{
						User: "root",
						Path: "/usr/bin/getfattr",
						Args: []string{"-d", "--absolute-names", "-m", "-", "-e", "hex", "/usr/bin/ping"},
					})
					Expect(exitCode).To(Equal(0))
					Expect(stderr).To(gbytes.Say("^$"))
					Expect(stdout).To(gbytes.Say("# file: /usr/bin/ping\nsecurity.capability=0x0100000200200000000000000000000000000000"))

				})
			})

			It("creates the files in the container, as the specified user", func() {
				if runtime.GOOS == "windows" {
					err := container.StreamIn(garden.StreamInSpec{
						Path:      "C:\\some-root",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())

					stdout := runForStdout(container, garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{"/C", "type", "C:\\some-root\\some-temp-dir\\some-temp-file"},
					})

					Expect(stdout).To(gbytes.Say("some-body"))

				} else {
					err := container.StreamIn(garden.StreamInSpec{
						User:      "alice",
						Path:      "/home/alice",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())

					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "test",
						Args: []string{"-f", "/home/alice/some-temp-dir/some-temp-file"},
					})
					Expect(exitCode).To(Equal(0))

					stdout := runForStdout(container, garden.ProcessSpec{
						User: "alice",
						Path: "ls",
						Args: []string{"-al", "/home/alice/some-temp-dir/some-temp-file"},
					})

					// output should look like -rwxrwxrwx 1 alice alice 9 Jan  1  1970 /tmp/some-container-dir/some-temp-dir/some-temp-file
					Expect(stdout).To(gbytes.Say("alice"))
					Expect(stdout).To(gbytes.Say("alice"))
				}
			})

			Context("when no user specified", func() {
				It("streams the files in as root", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					err := container.StreamIn(garden.StreamInSpec{
						Path:      "/home/alice",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())

					stdout := runForStdout(container, garden.ProcessSpec{
						User: "root",
						Path: "ls",
						Args: []string{"-la", "/home/alice/some-temp-dir/some-temp-file"},
					})
					Expect(stdout).To(gbytes.Say("root"))
				})
			})

			Context("when the destination is '/'", func() {
				It("does not fail", func() {
					err := container.StreamIn(garden.StreamInSpec{
						Path:      "/",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())
				})
			})

			Context("when a non-existent user specified", func() {
				It("returns error", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					err := container.StreamIn(garden.StreamInSpec{
						User:      "batman",
						Path:      "/home/alice",
						TarStream: tarStream,
					})
					Expect(err).To(MatchError(ContainSubstring("error streaming in")))
				})
			})

			Context("when the specified user does not have permission to stream in", func() {
				JustBeforeEach(func() {
					createUser(container, "bob")
				})

				It("returns error", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					err := container.StreamIn(garden.StreamInSpec{
						User:      "bob",
						Path:      "/home/alice",
						TarStream: tarStream,
					})
					Expect(err).To(MatchError(ContainSubstring("Permission denied")))
				})
			})

			Context("in a privileged container", func() {
				BeforeEach(func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					setPrivileged()
				})

				It("streams in relative to the default run directory", func() {
					err := container.StreamIn(garden.StreamInSpec{
						User:      "alice",
						Path:      ".",
						TarStream: tarStream,
					})
					Expect(err).ToNot(HaveOccurred())

					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "test",
						Args: []string{"-f", "some-temp-dir/some-temp-file"},
					})

					Expect(exitCode).To(Equal(0))
				})
			})

			It("streams in relative to the default run directory", func() {
				err := container.StreamIn(garden.StreamInSpec{
					User:      regularUser,
					Path:      ".",
					TarStream: tarStream,
				})
				Expect(err).ToNot(HaveOccurred())

				var spec garden.ProcessSpec
				if runtime.GOOS == "windows" {
					spec = garden.ProcessSpec{
						User: regularUser,
						Path: shell,
						Args: []string{"/C", "type", "some-temp-dir\\some-temp-file"},
					}
				} else {
					spec = garden.ProcessSpec{
						User: regularUser,
						Path: "test",
						Args: []string{"-f", "some-temp-dir/some-temp-file"},
					}
				}

				exitCode, _, _ := runProcess(container, spec)

				Expect(exitCode).To(Equal(0))
			})

			It("returns an error when the tar process dies", func() {
				err := container.StreamIn(garden.StreamInSpec{
					User: "alice",
					Path: "/tmp/some-container-dir",
					TarStream: &io.LimitedReader{
						R: tarStream,
						N: 10,
					},
				})
				Expect(err).To(HaveOccurred())
			})

			Context("and then copying them out", func() {
				itStreamsTheDirectory := func(user string) {
					It("streams the directory", func() {
						if runtime.GOOS == "windows" {
							exitCode, _, _ := runProcess(container, garden.ProcessSpec{
								User: regularUser,
								Path: shell,
								Args: []string{"/C", `mkdir some-outer-dir\some-inner-dir & echo body > some-outer-dir\some-inner-dir\some-file`},
							})
							Expect(exitCode).To(Equal(0))

							tarOutput, err := container.StreamOut(garden.StreamOutSpec{
								User: user,
								Path: fmt.Sprintf("c:\\users\\%s\\some-outer-dir\\some-inner-dir", regularUser),
							})
							Expect(err).ToNot(HaveOccurred())

							tarReader := tar.NewReader(tarOutput)

							header, err := tarReader.Next()
							Expect(err).ToNot(HaveOccurred())
							Expect(header.Name).To(Equal("some-inner-dir/"))

							header, err = tarReader.Next()
							Expect(err).ToNot(HaveOccurred())
							Expect(header.Name).To(Equal("some-inner-dir/some-file"))

						} else {
							exitCode, _, _ := runProcess(container, garden.ProcessSpec{
								User: "alice",
								Path: "sh",
								Args: []string{"-c", `mkdir -p some-outer-dir/some-inner-dir && touch some-outer-dir/some-inner-dir/some-file`},
							})

							Expect(exitCode).To(Equal(0))

							tarOutput, err := container.StreamOut(garden.StreamOutSpec{
								User: user,
								Path: "/home/alice/some-outer-dir/some-inner-dir",
							})
							Expect(err).ToNot(HaveOccurred())

							tarReader := tar.NewReader(tarOutput)

							header, err := tarReader.Next()
							Expect(err).ToNot(HaveOccurred())
							Expect(header.Name).To(Equal("some-inner-dir/"))

							header, err = tarReader.Next()
							Expect(err).ToNot(HaveOccurred())
							Expect(header.Name).To(Equal("some-inner-dir/some-file"))
						}
					})
				}

				itStreamsTheDirectory(regularUser)

				Context("when no user specified", func() {
					// Any user's files can be streamed out as root
					itStreamsTheDirectory("")
				})

				Context("with a trailing slash", func() {
					It("streams the contents of the directory", func() {
						var spec garden.ProcessSpec
						if runtime.GOOS == "windows" {
							spec = garden.ProcessSpec{
								User: regularUser,
								Path: shell,
								Args: []string{"/C", "mkdir some-container-dir & echo body > some-container-dir\\some-file"},
							}
						} else {
							spec = garden.ProcessSpec{
								User: regularUser,
								Path: shell,
								Args: []string{"-c", `mkdir -p some-container-dir && touch some-container-dir/some-file`},
							}
						}
						exitCode, _, _ := runProcess(container, spec)

						Expect(exitCode).To(Equal(0))

						tarOutput, err := container.StreamOut(garden.StreamOutSpec{
							User: regularUser,
							Path: "some-container-dir/",
						})
						Expect(err).ToNot(HaveOccurred())

						tarReader := tar.NewReader(tarOutput)

						header, err := tarReader.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(header.Name).To(Equal("./"))

						header, err = tarReader.Next()
						Expect(err).ToNot(HaveOccurred())
						Expect(header.Name).To(Equal("./some-file"))
					})
				})
			})
		})
	})

	Context("when the container GraceTime is applied", func() {
		skipIfWoot("Groot does not support deleting containers yet")

		It("should disappear after grace time and before timeout", func() {
			containerHandle := container.Handle()
			Expect(container.SetGraceTime(500 * time.Millisecond)).To(Succeed())

			_, err := gardenClient.Lookup(containerHandle)
			Expect(err).NotTo(HaveOccurred())
			container = nil // avoid double-destroying in AfterEach

			if runtime.GOOS == "windows" {
				Eventually(func() error {
					_, err := gardenClient.Lookup(containerHandle)
					return err
				}, "60s", "2s").Should(HaveOccurred())
			} else {
				Eventually(func() error {
					_, err := gardenClient.Lookup(containerHandle)
					return err
				}, "60s", "2s").Should(HaveOccurred())
			}
		})

		It("returns an unknown handle error when calling the API", func() {
			Eventually(func() error {
				return gardenClient.Destroy("not-a-real-handle")
			}).Should(MatchError(fmt.Sprintf("unknown handle: %s", "not-a-real-handle")))
		})

		Context("when a process is started", func() {
			Context("and the container GraceTime is reset", func() {
				It("should account for existing client connections", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					processSpec := garden.ProcessSpec{
						Path: "sh",
						Args: []string{"-c", `/bin/sleep 1000`},
					}
					stdOut, stdErr := gbytes.NewBuffer(), gbytes.NewBuffer()
					_, err := container.Run(
						processSpec,
						garden.ProcessIO{
							Stdout: io.MultiWriter(stdOut, GinkgoWriter),
							Stderr: io.MultiWriter(stdErr, GinkgoWriter),
						})
					Expect(err).NotTo(HaveOccurred())

					Expect(container.SetGraceTime(50 * time.Millisecond)).To(Succeed())
					Consistently(func() error {
						_, err := gardenClient.Lookup(container.Handle())
						return err
					}, "1s", "1s").ShouldNot(HaveOccurred())
				})
			})
		})
	})
})
