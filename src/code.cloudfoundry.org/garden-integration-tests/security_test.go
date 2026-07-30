package garden_integration_tests_test

import (
	"regexp"
	"runtime"
	"strings"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Security", func() {
	var (
		peaImage garden.ImageRef
		noImage  garden.ImageRef
	)

	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
		peaImage = garden.ImageRef{URI: gardenRootfs}
		noImage = garden.ImageRef{}
	})

	Describe("PID namespace", func() {
		BeforeEach(func() {
			imageRef.URI = limitsTestURI
		})
		It("isolates processes so that only processes from inside the container are visible", func() {
			createUser(container, "alice")

			_, err := container.Run(garden.ProcessSpec{
				User: "alice",
				Path: "/bin/sleep",
				Args: []string{"989898"},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).ToNot(HaveOccurred())

			Eventually(func() []string {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", "ps -a"},
				})

				return strings.Split(string(stdout.Contents()), "\n")
			}).Should(HaveLen(5)) // header, garden-init, sleep, ps, \n
		})

		It("does not leak fds in to spawned processes", func() {

			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "ls",
				Args: []string{"/proc/self/fd"},
			})

			Expect(stdout).To(gbytes.Say("0\n1\n2\n3\n")) // stdin, stdout, stderr, /proc/self/fd
		})
	})

	Describe("File system", func() {
		It("/tmp is world-writable in the container", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "ls",
				Args: []string{"-al", "/tmp"},
			})
			Expect(stdout).To(gbytes.Say(`drwxrwxrwt`))
		})

		It("/tmp IS mounted as tmpfs", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "cat",
				Args: []string{"/proc/mounts"},
			})
			Expect(stdout).To(gbytes.Say("tmpfs /dev/shm tmpfs"))
		})

		Context("in an unprivileged container", func() {
			BeforeEach(func() {
				privilegedContainer = false
			})

			It("/sys IS mounted as Read-Only", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "cat",
					Args: []string{"/proc/mounts"},
				})
				Expect(stdout).To(gbytes.Say("sysfs /sys sysfs ro"))
			})

			Describe("for cgroups-v1", func() {
				BeforeEach(func() {
					if cgroups.IsCgroup2UnifiedMode() {
						Skip("skiping for cgroups-v2")
					}
				})

				It("cgroup filesystems are mounted as read-only", func() {
					getCGroupMountOptions := func(mounts string) map[string]bool {
						cgroupMountRegex := regexp.MustCompile(`.*\s/sys/fs/cgroup/[^\s]*\s([^\s]*).*cgroup cgroup *\s([^\s]*)`)
						matches := cgroupMountRegex.FindAllStringSubmatch(mounts, -1)
						cgroupMountOptions := make(map[string]bool)
						for _, m := range matches {
							if len(m) == 3 && strings.Contains(m[1], "ro") {
								for _, option := range strings.Split(m[2], ",") {
									cgroupMountOptions[option] = true
								}
							}
						}
						return cgroupMountOptions
					}

					stdout := runForStdout(container, garden.ProcessSpec{
						User: "root",
						Path: "grep",
						Args: []string{"cgroup", "/proc/self/mountinfo"},
					})

					cgroupMountOptions := getCGroupMountOptions(string(stdout.Contents()))
					Expect(cgroupMountOptions).To(HaveKey("cpu"))
					Expect(cgroupMountOptions).To(HaveKey("cpuacct"))
					Expect(cgroupMountOptions).To(HaveKey("net_cls"))
					Expect(cgroupMountOptions).To(HaveKey("net_prio"))
					Expect(cgroupMountOptions).To(HaveKey("memory"))
					Expect(cgroupMountOptions).To(HaveKey("cpuset"))
					Expect(cgroupMountOptions).To(HaveKey("blkio"))
					Expect(cgroupMountOptions).To(HaveKey("devices"))
					Expect(cgroupMountOptions).To(HaveKey("freezer"))
					Expect(cgroupMountOptions).To(HaveKey("perf_event"))
					Expect(cgroupMountOptions).To(HaveKey("pids"))
					Expect(cgroupMountOptions).To(HaveKey("hugetlb"))
				})
			})
		})

		Context("in a privileged container", func() {
			BeforeEach(func() {
				setPrivileged()
			})

			It("/proc IS mounted as Read-Write", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "cat",
					Args: []string{"/proc/mounts"},
				})
				Expect(stdout).To(gbytes.Say("proc /proc proc rw"))
			})

			It("/sys IS mounted as Read-Only", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "cat",
					Args: []string{"/proc/mounts"},
				})
				Expect(stdout).To(gbytes.Say("sysfs /sys sysfs ro"))
			})

			It("cgroup filesystems are not mounted", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "cat",
					Args: []string{"/proc/mounts"},
				})

				Expect(stdout).NotTo(gbytes.Say("cgroup"))
			})
		})
	})

	Describe("Control groups", func() {
		BeforeEach(func() {
			if cgroups.IsCgroup2UnifiedMode() {
				Skip("this test is for cgroups-v1")
			}
		})
		It("places the container in the required cgroup subsystems", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "/bin/sh",
				Args: []string{"-c", "cat /proc/$$/cgroup"},
			})

			op := stdout.Contents()
			Expect(op).To(MatchRegexp(`\bcpu\b`))
			Expect(op).To(MatchRegexp(`\bcpuacct\b`))
			Expect(op).To(MatchRegexp(`\bcpuset\b`))
			Expect(op).To(MatchRegexp(`\bdevices\b`))
			Expect(op).To(MatchRegexp(`\bmemory\b`))
		})
	})

	Describe("rlimits", func() {
		BeforeEach(func() {
			imageRef.URI = limitsTestURI
		})
		It("sets requested rlimits", func() {
			limit := uint64(4567)
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "/bin/sh",
				Args: []string{"-c", "ulimit -a"},
				Limits: garden.ResourceLimits{
					Nproc: &limit,
				},
			})
			Expect(stdout).To(gbytes.Say("processes\\W+(-u)\\W+4567"))
		})
	})

	Describe("Users and groups", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		JustBeforeEach(func() {
			createUser(container, "alice")
		})

		It("maintains setuid permissions in unprivileged containers", func() {
			skipIfWoot("Woot blindly chmods to Maximus thus screwing the suid flag")
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "alice",
				Path: "ls",
				Args: []string{"-l", "/bin/usemem-with-setuid"},
			})
			Eventually(stdout).Should(gbytes.Say("-rws"))
		})

		Context("when running a command in a working dir", func() {
			It("executes with setuid and setgid", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Dir:  "/usr",
					Path: "pwd",
				})
				Expect(stdout).To(gbytes.Say("^/usr\n"))
			})
		})

		Context("when running a command as a non-root user", func() {
			It("executes with correct uid, gid, and supplementary gids", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "/bin/sh",
					Args: []string{"-c", "id -u; id -g; id -G"},
				})
				Expect(stdout).To(gbytes.Say("1001\n1010\n1010 1011\n"))
			})

			It("sets $HOME, $USER, and $PATH", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				})
				Expect(stdout).To(gbytes.Say("HOME=/home/alice\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/bin:/from-dockerfile\nPWD=/home/alice\nSHLVL=1\nTEST=second-test-from-dockerfile:test-from-dockerfile\nUSER=alice\n"))
			})

			Context("when $HOME is set in the spec", func() {
				It("sets $HOME from the spec", func() {
					stdout := runForStdout(container, garden.ProcessSpec{
						User: "alice",
						Path: "/bin/sh",
						Args: []string{"-c", "echo $HOME"},
						Env: []string{
							"HOME=/nowhere",
						},
					})
					Expect(stdout).To(gbytes.Say("/nowhere"))
				})
			})

			Context("when env is set in the spec", func() {
				It("sets env from the spec", func() {
					stdout := runForStdout(container, garden.ProcessSpec{
						User: "alice",
						Path: "/bin/sh",
						Args: []string{"-c", "env"},
						Env: []string{
							"USER=nobody",
						},
					})
					Expect(stdout).To(gbytes.Say("USER=nobody"))
					Expect(stdout).To(gbytes.Say("HOME=/home/alice"))
				})
			})

			It("executes in the user's home directory", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "/bin/pwd",
				})
				Expect(stdout).To(gbytes.Say("/home/alice\n"))
			})

			It("searches a sanitized path not including /sbin for the executable", func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "alice",
					Path: "ls",
				})
				Expect(exitCode).To(Equal(0))

				_, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "hello-world", // hello-world is only available in /sbin
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})
				Expect(err).To(HaveOccurred())
			})
		})

		Context("when running a command as root", func() {
			It("executes with uid 0, gid 0, and supplementary gids from /etc/group", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "id -u; id -g; id -G"},
				})
				Expect(stdout).To(gbytes.Say("0\n0\n0 10\n"))
			})

			It("sets $HOME, $USER, and $PATH", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "env | sort"},
				})
				Expect(stdout).To(gbytes.Say("HOME=/root\nPATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/bin:/from-dockerfile\nPWD=/root\nSHLVL=1\nTEST=second-test-from-dockerfile:test-from-dockerfile\nUSER=root\n"))
			})

			It("executes in root's home directory", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "/bin/pwd",
				})
				Expect(stdout).To(gbytes.Say("/root\n"))
			})

			It("searches a sanitized path not including /sbin for the executable", func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "hello-world", // hello-world is only available in /sbin
					Env:  []string{"PATH=/sbin"},
				})
				Expect(exitCode).To(Equal(0))
			})
		})
	})

	Context("by default (unprivileged)", func() {
		Describe("seccomp", func() {
			itAppliesSeccomp := func(image garden.ImageRef) {
				It("blocks syscalls not whitelisted in the default seccomp profile", func() {
					exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
						Path:  "unshare",
						Args:  []string{"--user", "whoami"},
						Image: image,
					})

					Expect(exitCode).NotTo(Equal(0))
					Expect(stderr).To(gbytes.Say("Operation not permitted"))
				})

				It("applies seccomp in filter mode", func() {
					stdout := runForStdout(container, garden.ProcessSpec{
						Path:  "grep",
						Args:  []string{"Seccomp", "/proc/self/status"},
						Image: image,
					})
					Expect(string(stdout.Contents())).To(MatchRegexp(`Seccomp:\s+2`))
				})
			}

			itAppliesSeccomp(noImage)

			Context("when running a pea", func() {
				itAppliesSeccomp(peaImage)
			})
		})

		It("does not get root privileges on host resources", func() {
			exitCode, _, _ := runProcess(container, garden.ProcessSpec{
				Path: "sh",
				User: "root",
				Args: []string{"-c", "echo h > /proc/sysrq-trigger"},
			})
			Expect(exitCode).ToNot(Equal(0))
		})

		It("can write to files in the /root directory", func() {
			exitCode, _, _ := runProcess(container, garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{"-c", `touch /root/potato`},
			})
			Expect(exitCode).To(Equal(0))
		})

		Describe("capabilities", func() {
			Describe("the init process", func() {
				It("has a reduced set of capabilities, not including CAP_SYS_ADMIN", func() {
					stdout := runForStdout(container, garden.ProcessSpec{
						Path: "cat",
						Args: []string{"/proc/1/status"},
					})
					Eventually(stdout).Should(gbytes.Say("CapInh:\\W+00000000a80425fb"))
					Eventually(stdout).Should(gbytes.Say("CapPrm:\\W+00000000a80425fb"))
					Eventually(stdout).Should(gbytes.Say("CapEff:\\W+00000000a80425fb"))
					Eventually(stdout).Should(gbytes.Say("CapBnd:\\W+00000000a80425fb"))
					Eventually(stdout).Should(gbytes.Say("CapAmb:\\W+0000000000000000"))
				})
			})

			Describe("a process running as the root user", func() {
				itHasReducedCapabilities := func(image garden.ImageRef) {
					It("has a reduced set of capabilities, not including CAP_SYS_ADMIN", func() {
						stdout := runForStdout(container, garden.ProcessSpec{
							Path:  "cat",
							Args:  []string{"/proc/self/status"},
							Image: image,
						})
						Eventually(stdout).Should(gbytes.Say("CapInh:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapPrm:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapEff:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapBnd:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapAmb:\\W+0000000000000000"))
					})
				}

				itHasReducedCapabilities(noImage)

				Context("when running a pea", func() {
					itHasReducedCapabilities(peaImage)
				})
			})

			Describe("a process running as a non-root user", func() {
				itHasCorrectCapabilities := func(image garden.ImageRef) {
					It("it has no effective caps and a reduced set of bounding capabilities, not including CAP_SYS_ADMIN", func() {
						stdout := runForStdout(container, garden.ProcessSpec{
							User:  "1000:1000",
							Path:  "cat",
							Args:  []string{"/proc/self/status"},
							Image: image,
						})

						Eventually(stdout).Should(gbytes.Say("CapInh:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapPrm:\\W+0000000000000000"))
						Eventually(stdout).Should(gbytes.Say("CapEff:\\W+0000000000000000"))
						Eventually(stdout).Should(gbytes.Say("CapBnd:\\W+00000000a80425fb"))
						Eventually(stdout).Should(gbytes.Say("CapAmb:\\W+0000000000000000"))
					})
				}

				itHasCorrectCapabilities(noImage)

				Context("when running a pea", func() {
					itHasCorrectCapabilities(peaImage)
				})
			})
		})

		Context("with a docker image", func() {
			BeforeEach(func() {
				imageRef.URI = "docker:///cfgarden/preexisting_users"
			})

			It("sees root-owned files in the rootfs as owned by the container's root user", func() {
				skipIfWoot("Woot blindly chmods to Maximus thus screwing the suid flag")
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", `ls -l /bin | grep -v wsh | grep -v hook | grep -v proc_starter | grep -v initd`},
				})
				Expect(stdout).NotTo(gbytes.Say("nobody"))
				Expect(stdout).NotTo(gbytes.Say("65534"))
				Expect(stdout).To(gbytes.Say(" root "))
			})

			It("sees the /dev/pts and /dev/ptmx as owned by the container's root user", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", "ls -l /dev/pts /dev/ptmx /dev/pts/ptmx"},
				})

				Expect(stdout).NotTo(gbytes.Say("nobody"))
				Expect(stdout).NotTo(gbytes.Say("65534"))
				Expect(stdout).To(gbytes.Say(" root "))
			})

			It("sees alice-owned files as owned by alice", func() {
				skipIfWoot("Woot blindly chmods to Maximus thus screwing the suid flag")
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", `ls -la /home/alice`},
				})
				Expect(stdout).To(gbytes.Say(" alice "))
				Expect(stdout).To(gbytes.Say(" alicesfile"))
			})

			It("lets alice write in /home/alice", func() {
				skipIfWoot("Woot blindly chmods to Maximus thus screwing the suid flag")
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "alice",
					Path: "touch",
					Args: []string{"/home/alice/newfile"},
				})
				Expect(exitCode).To(Equal(0))
			})

			It("lets root write to files in the /root directory", func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", `touch /root/potato`},
				})
				Expect(exitCode).To(Equal(0))
			})

			It("preserves pre-existing dotfiles from base image", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "cat",
					Args: []string{"/.foo"},
				})
				Expect(stdout).To(gbytes.Say("this is a pre-existing dotfile"))
			})
		})
	})

	Context("when the 'privileged' flag is set on the create call", func() {
		BeforeEach(func() {
			setPrivileged()
		})

		Context("and the user is root", func() {
			It("has a full set of capabilities", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					Path: "cat",
					Args: []string{"/proc/self/status"},
				})

				Expect(stdout).To(gbytes.Say("CapInh:\\W+0000003fffffffff"))
				Expect(stdout).To(gbytes.Say("CapPrm:\\W+0000003fffffffff"))
				Expect(stdout).To(gbytes.Say("CapEff:\\W+0000003fffffffff"))
				Expect(stdout).To(gbytes.Say("CapBnd:\\W+0000003fffffffff"))
				Expect(stdout).To(gbytes.Say("CapAmb:\\W+0000000000000000"))
			})
		})

		Context("and the user is not root", func() {
			JustBeforeEach(func() {
				createUser(container, "alice")
			})

			It("has no effective capabilities, and a reduced set of capabilities that does include CAP_SYS_ADMIN", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "alice",
					Path: "cat",
					Args: []string{"/proc/self/status"},
				})

				Expect(stdout).To(gbytes.Say("CapInh:\\W+00000000a82425fb"))
				Expect(stdout).To(gbytes.Say("CapPrm:\\W+0000000000000000"))
				Expect(stdout).To(gbytes.Say("CapEff:\\W+0000000000000000"))
				Expect(stdout).To(gbytes.Say("CapBnd:\\W+00000000a82425fb"))
				Expect(stdout).To(gbytes.Say("CapAmb:\\W+0000000000000000"))
			})
		})

		It("can write to files in the /root directory", func() {
			exitCode, _, _ := runProcess(container, garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{"-c", `touch /root/potato`},
			})
			Expect(exitCode).To(Equal(0))
		})

		It("sees root-owned files in the rootfs as owned by the container's root user", func() {
			stdout := runForStdout(container, garden.ProcessSpec{
				User: "root",
				Path: "sh",
				Args: []string{"-c", `ls -l /bin | grep -v wsh | grep -v hook`},
			})
			Expect(stdout).NotTo(gbytes.Say("nobody"))
			Expect(stdout).NotTo(gbytes.Say("65534"))
			Expect(stdout).To(gbytes.Say(" root "))
		})

		Context("when the process is run as non-root user", func() {
			BeforeEach(func() {
				skipIfWoot("Ubuntu docker image layers contain whiteouts and woot does not handle them properly")
				imageRef.URI = "docker:///ubuntu#14.04"
			})

			Context("and the user changes to root", func() {
				JustBeforeEach(func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "root",
						Path: "sh",
						Args: []string{"-c", `echo "ALL            ALL = (ALL) NOPASSWD: ALL" >> /etc/sudoers`},
					})

					Expect(exitCode).To(Equal(0))

					exitCode, _, _ = runProcess(container, garden.ProcessSpec{
						User: "root",
						Path: "useradd",
						Args: []string{"-U", "-m", "bob"},
					})
					Expect(exitCode).To(Equal(0))
				})

				It("can chown files", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "bob",
						Path: "sudo",
						Args: []string{"chown", "-R", "bob", "/tmp"},
					})

					Expect(exitCode).To(Equal(0))
				})

				It("does not have certain capabilities", func() {
					// This attempts to set system time which requires the CAP_SYS_TIME permission.
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "bob",
						Path: "sudo",
						Args: []string{"date", "--set", "+2 minutes"},
					})

					Expect(exitCode).ToNot(Equal(0))
				})
			})
		})
	})
})
