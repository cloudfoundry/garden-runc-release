package garden_integration_tests_test

import (
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Process", func() {
	Describe("signalling", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		It("a process can be sent SIGTERM immediately after having been started", func() {
			stdout := gbytes.NewBuffer()

			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{
					"-c",
					`
                /bin/sleep 10
                exit 12
                `,
				},
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).ToNot(HaveOccurred())

			err = process.Signal(garden.SignalTerminate)
			Expect(err).ToNot(HaveOccurred())
			Expect(process.Wait()).NotTo(Equal(12))
		})
	})

	Describe("when we try to create a container process with bind mounts", func() {
		It("should return an error", func() {
			stdout := gbytes.NewBuffer()

			_, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "whoami",
				Args: []string{},
				BindMounts: []garden.BindMount{
					garden.BindMount{
						SrcPath: "src",
						DstPath: "dst",
					},
				},
			}, garden.ProcessIO{
				Stdout: stdout,
			})
			Expect(err).To(HaveOccurred())
		})
	})
	Describe("process ID", func() {
		It("return a process containing the ID passed in the process spec", func() {
			var spec garden.ProcessSpec
			if runtime.GOOS == "windows" {
				spec = garden.ProcessSpec{
					ID:   "some-id",
					Path: "whoami",
				}
			} else {
				spec = garden.ProcessSpec{
					ID:   "some-id",
					Path: "/bin/true",
				}
			}
			process, err := container.Run(spec, garden.ProcessIO{})
			Expect(err).ToNot(HaveOccurred())
			Expect(process.ID()).To(Equal("some-id"))
		})

		Context("when two processes with the same ID are running", func() {
			var processID string

			JustBeforeEach(func() {
				processID = "same-id"
				var spec garden.ProcessSpec
				if runtime.GOOS == "windows" {
					spec = garden.ProcessSpec{
						ID:   processID,
						Path: "cmd.exe",
						Args: []string{"/C", "waitfor five_sec /T 5 & exit /b 0"},
					}
				} else {
					spec = garden.ProcessSpec{
						ID:   processID,
						Path: "/bin/sleep",
						Args: []string{"5"},
					}
				}
				_, err := container.Run(spec, garden.ProcessIO{})
				Expect(err).ToNot(HaveOccurred())
			})

			It("the second process with the same id should explode", func() {
				var spec garden.ProcessSpec
				if runtime.GOOS == "windows" {
					spec = garden.ProcessSpec{
						ID:   processID,
						Path: "whoami",
					}
				} else {
					spec = garden.ProcessSpec{
						ID:   processID,
						Path: "/bin/true",
					}
				}
				_, err := container.Run(spec, garden.ProcessIO{})
				Expect(err).To(MatchError(MatchRegexp(`already (in use|exists)`)))
			})
		})
	})

	Describe("environment", func() {
		It("should apply the specified environment", func() {
			var spec garden.ProcessSpec
			if runtime.GOOS == "windows" {
				spec = garden.ProcessSpec{
					Path: "cmd.exe",
					Args: []string{"/C", "set"},
					Env: []string{
						"TEST=hello",
						"FRUIT=banana",
					},
				}
				exitCode, stdout, _ := runProcess(container, spec)
				Expect(exitCode).To(Equal(0))
				Expect(stdout.Contents()).To(ContainSubstring("TEST=hello\r\n"))
				Expect(stdout.Contents()).To(ContainSubstring("FRUIT=banana\r\n"))
			} else {
				spec = garden.ProcessSpec{
					Path: "env",
					Env: []string{
						"TEST=hello",
						"FRUIT=banana",
					},
				}
				exitCode, stdout, _ := runProcess(container, spec)
				Expect(exitCode).To(Equal(0))
				Expect(stdout).To(gbytes.Say("TEST=hello\nFRUIT=banana"))
			}
		})

		Context("when the container has container spec environment specified", func() {
			BeforeEach(func() {
				env = []string{
					"CONTAINER_ENV=1",
					"TEST=hi",
				}
			})

			It("should apply the merged environment variables", func() {
				if runtime.GOOS == "windows" {

					exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
						Path: "cmd.exe",
						Args: []string{"/C", "set"},
						Env: []string{
							"TEST=hello",
							"FRUIT=banana",
						},
					})
					Expect(exitCode).To(Equal(0))
					Expect(stdout.Contents()).To(ContainSubstring("CONTAINER_ENV=1\r\n"))
					Expect(stdout.Contents()).To(ContainSubstring("TEST=hello\r\n"))
					Expect(stdout.Contents()).To(ContainSubstring("FRUIT=banana\r\n"))
				} else {
					exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
						Path: "env",
						Env: []string{
							"TEST=hello",
							"FRUIT=banana",
						},
					})
					Expect(exitCode).To(Equal(0))
					Expect(stdout).To(gbytes.Say("CONTAINER_ENV=1\nTEST=hello\nFRUIT=banana"))
				}
			})
		})
	})

	Describe("wait", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		It("does not block in Wait() when all children of the process have exited", func() {
			stderr := gbytes.NewBuffer()
			process, err := container.Run(garden.ProcessSpec{
				User: "root",
				Path: "/bin/sh",
				Args: []string{"-c", `

				  cleanup ()
				  {
						ps -a >&2
						kill $child_pid
						exit 42
				  }

				  trap cleanup TERM
				  set -x
				  /bin/sleep 1000 &
				  child_pid=$!
				  # Make sure that sleep process has been forked before trapping
				  while [ ! $(ps -o comm | grep sleep) ] ;do : ; done
				  # Use stderr to avoid buffering in the shell.
				  echo trapping >&2
				  wait
				`},
			}, garden.ProcessIO{Stderr: stderr})
			Expect(err).NotTo(HaveOccurred())

			exitChan := make(chan int)
			go func(p garden.Process, exited chan<- int) {
				defer GinkgoRecover()
				status, waitErr := p.Wait()
				Expect(waitErr).NotTo(HaveOccurred())
				exited <- status
			}(process, exitChan)

			Eventually(stderr).Should(gbytes.Say("trapping"))

			Expect(process.Signal(garden.SignalTerminate)).To(Succeed())

			select {
			case status := <-exitChan:
				Expect(status).To(Equal(42))
			case <-time.After(time.Second * 20):
				debug.PrintStack()
				Expect(err).NotTo(HaveOccurred())
				fmt.Printf("Process Stderr: %s", string(stderr.Contents()))
				Fail("timed out!")
			}
		})
	})

	Describe("user", func() {
		Context("when the user is specified in the form uid:gid", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
			})
			It("runs the process as that user", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "1001:1002",
					Path: "sh",
					Args: []string{"-c", "echo $(id -u):$(id -g)"},
				})
				Expect(stdout).To(gbytes.Say("1001:1002\n"))
			})
		})

		Context("when the user is specified ins the form username:groupname", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				imageRef = garden.ImageRef{
					URI: "docker:///cloudfoundry/garden-rootfs",
				}
			})

			It("runs the process as that user", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "testuser:staff",
					Path: "sh",
					Args: []string{"-c", "echo $(id -u):$(id -g)"},
				})
				Expect(stdout).To(gbytes.Say("1000:50\n"))
			})
		})

		Context("when the user is not specified", func() {
			It("runs the process as root", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					Path: "whoami",
				})
				if runtime.GOOS == "windows" {
					Expect(stdout).To(gbytes.Say("containeradministrator"))
				} else {
					Expect(stdout).To(gbytes.Say("root\n"))
				}
			})
		})
	})

	Describe("working directory", func() {
		JustBeforeEach(func() {
			createUser(container, "alice")
		})

		Context("when user has access to working directory", func() {
			Context("when working directory exists", func() {
				It("spawns the process", func() {
					if runtime.GOOS == "windows" {
						stdout := runForStdout(container, garden.ProcessSpec{
							User: "alice",
							Dir:  "c:\\users\\alice",
							Path: "cmd.exe",
							Args: []string{"/C", `echo %cd%`},
						})

						Expect(stdout).To(gbytes.Say(`c:\\users\\alice`))
					} else {
						stdout := runForStdout(container, garden.ProcessSpec{
							User: "alice",
							Dir:  "/home/alice",
							Path: "pwd",
						})

						Expect(stdout).To(gbytes.Say("/home/alice"))
					}
				})
			})

			Context("when working directory does not exist", func() {
				It("spawns the process", func() {
					if runtime.GOOS == "windows" {
						stdout := runForStdout(container, garden.ProcessSpec{
							User: "alice",
							Dir:  "c:\\users\\alice\\nonexistent",
							Path: "cmd.exe",
							Args: []string{"/C", `echo %cd%`},
						})

						Expect(stdout.Contents()).To(ContainSubstring("c:\\users\\alice\\nonexistent"))
					} else {
						stdout := runForStdout(container, garden.ProcessSpec{
							User: "alice",
							Dir:  "/home/alice/nonexistent",
							Path: "pwd",
						})

						Expect(stdout).To(gbytes.Say("/home/alice/nonexistent"))
					}
				})

				It("is created owned by the requested user", func() {
					if runtime.GOOS == "windows" {
						Skip("pending for windows")
					}
					stdout := runForStdout(container, garden.ProcessSpec{
						User: "root",
						Dir:  "/root/nonexistent",
						Path: "sh",
						Args: []string{"-c", "ls -la . | head -n 2 | tail -n 1"},
					})

					Expect(stdout).To(gbytes.Say("root"))
				})
			})
		})

		Context("when user does not have access to working directory", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
			})
			JustBeforeEach(func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", "mkdir -p /home/alice/nopermissions && chmod 0555 /home/alice/nopermissions"},
				})
				Expect(exitCode).To(Equal(0))
			})

			Context("when working directory does exist", func() {
				It("returns an error", func() {
					exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Dir:  "/home/alice/nopermissions",
						Path: "touch",
						Args: []string{"test.txt"},
					})

					Expect(exitCode).ToNot(Equal(0))
					Expect(stderr).To(gbytes.Say("Permission denied"))
				})
			})

			Context("when working directory does not exist", func() {
				It("should create the working directory, and succeed", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Dir:  "/home/alice/nopermissions/nonexistent",
						Path: "touch",
						Args: []string{"test.txt"},
					})

					Expect(exitCode).To(Equal(0))
				})
			})
		})

		Context("when the user does not specify the working directory", func() {
			It("should have the user home directory in the output", func() {
				if runtime.GOOS == "windows" {
					stdout := runForStdout(container, garden.ProcessSpec{
						User: "alice",
						Path: "cmd.exe",
						Args: []string{"/C", `echo %cd%`},
					})

					Expect(stdout).To(gbytes.Say(`C:\\Users\\alice`))
				} else {
					stdout := runForStdout(container, garden.ProcessSpec{
						User: "alice",
						Path: "pwd",
					})

					Expect(stdout).To(gbytes.Say("/home/alice"))
				}
			})
		})
	})
})
