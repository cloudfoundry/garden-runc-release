package garden_integration_tests_test

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Limits", func() {
	Describe("cgroups-v1 CPU limits", func() {
		BeforeEach(func() {
			if cgroups.IsCgroup2UnifiedMode() {
				Skip("running this for cgroups-v1 only")
			}
			limits.CPU = garden.CPULimits{
				LimitInShares: 100,
			}
		})

		It("reports the CPU limit", func() {
			cpuLimits, err := container.CurrentCPULimits()
			Expect(err).NotTo(HaveOccurred())
			//lint:ignore SA1019 - we still specify this to make the deprecated logic work until we get rid of the code in garden
			Expect(cpuLimits.LimitInShares).To(BeEquivalentTo(100))
		})
	})

	Describe("cgroups-v2 CPU limits", func() {
		BeforeEach(func() {
			if !cgroups.IsCgroup2UnifiedMode() {
				Skip("running this for cgroups-v2 only")
			}
			limits.CPU = garden.CPULimits{
				Weight: 280,
			}
		})

		It("reports the CPU limit", func() {
			cpuLimits, err := container.CurrentCPULimits()
			Expect(err).NotTo(HaveOccurred())
			//lint:ignore SA1019 - we still specify this to make the deprecated logic work until we get rid of the code in garden
			Expect(cpuLimits.LimitInShares).To(BeEquivalentTo(cgroups.ConvertCPUSharesToCgroupV2Value(limits.CPU.Weight)))
		})
	})

	Describe("memory limits", func() {
		var tarStream io.Reader
		BeforeEach(func() {
			var memLimit garden.MemoryLimits
			if runtime.GOOS == "windows" {
				memLimit = garden.MemoryLimits{
					LimitInBytes: 150 * 1024 * 1024,
				}

			} else {
				memLimit = garden.MemoryLimits{
					LimitInBytes: 64 * 1024 * 1024,
				}
			}
			limits = garden.Limits{Memory: memLimit}
		})

		JustBeforeEach(func() {
			if runtime.GOOS == "windows" {
				tmpdir, err := os.MkdirTemp("", "")
				Expect(err).ToNot(HaveOccurred())

				tgzPath := filepath.Join(tmpdir, "consume.tgz")
				contents, err := os.ReadFile(consumeBin)
				Expect(err).To(Succeed())

				createTarGZArchive(
					tgzPath,
					[]archiveFile{
						{
							Name: "./consume.exe",
							Body: contents,
						},
					},
				)

				tgz, err := os.Open(tgzPath)
				Expect(err).ToNot(HaveOccurred())

				tarStream, err = gzip.NewReader(tgz)
				Expect(err).ToNot(HaveOccurred())

				err = container.StreamIn(garden.StreamInSpec{
					Path:      "C:\\",
					TarStream: tarStream,
				})
				Expect(err).ToNot(HaveOccurred())
			}
		})

		It("reports the memory limits", func() {
			memoryLimits, err := container.CurrentMemoryLimits()
			Expect(err).NotTo(HaveOccurred())
			if runtime.GOOS == "windows" {
				Expect(memoryLimits.LimitInBytes).To(BeEquivalentTo(150 * 1024 * 1024))
			} else {
				Expect(memoryLimits.LimitInBytes).To(BeEquivalentTo(64 * 1024 * 1024))
			}
		})

		It("kills a process if it uses too much memory", func() {
			if runtime.GOOS == "windows" {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					Path: "C:\\consume.exe",
					Args: []string{strconv.Itoa(150 * 1024 * 1024)},
				})
				Expect(exitCode).To(Equal(2))
			} else {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "dd",
					Args: []string{"if=/dev/urandom", "of=/dev/shm/too-big", "bs=1M", "count=65"},
				})

				Expect(exitCode).ToNot(Equal(0))
			}
		})

		It("doesn't kill a process that uses lots of memory within the limit", func() {
			if runtime.GOOS == "windows" {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					Path: "C:\\consume.exe",
					Args: []string{strconv.Itoa(50 * 1024 * 1024)},
				})
				Expect(exitCode).To(Equal(0))
			} else {
				// Note: the dd process itself takes up a certain amount of memory, so we have to
				// allow for that overhead. This is why we don't E.g. write 63MB.
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "dd",
					Args: []string{"if=/dev/urandom", "of=/dev/shm/almost-too-big", "bs=1M", "count=58"},
				})
				Expect(exitCode).To(Equal(0))
			}
		})
	})

	Describe("disk limits", func() {
		BeforeEach(func() {
			skipIfWoot("Groot does not support disk size limits yet")
			privilegedContainer = false

			if runtime.GOOS == "windows" {
				limits.Disk.ByteHard = 100 * 1024 * 1024
			} else {
				limits.Disk.ByteSoft = 500 * 1024 * 1024
				limits.Disk.ByteHard = 500 * 1024 * 1024
				limits.Disk.Scope = garden.DiskLimitScopeTotal
			}
		})
		Context("Validating Metrics", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
			})

			DescribeTable("Metrics",
				func(reporter func() uint64) {
					createUser(container, "alice")

					initialBytes := reporter()
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/alice/some-file", "bs=1M", "count=3"},
					})
					Expect(exitCode).To(Equal(0))

					Eventually(reporter).Should(BeNumerically("~", initialBytes+3*1024*1024, 1024*1024))

					exitCode, _, _ = runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/alice/another-file", "bs=1M", "count=10"},
					})
					Expect(exitCode).To(Equal(0))

					Eventually(reporter).Should(BeNumerically("~", initialBytes+uint64(13*1024*1024), 1024*1024))
				},

				Entry("with exclusive metrics", func() uint64 {
					metrics, err := container.Metrics()
					Expect(err).ToNot(HaveOccurred())
					return metrics.DiskStat.ExclusiveBytesUsed
				}),

				Entry("with total metrics", func() uint64 {
					metrics, err := container.Metrics()
					Expect(err).ToNot(HaveOccurred())
					return metrics.DiskStat.TotalBytesUsed
				}),
			)
		})

		Context("when the scope is total", func() {
			BeforeEach(func() {

				imageRef.URI = limitsTestURI
				if runtime.GOOS == "windows" {
					limits.Disk.ByteHard = limitsTestContainerImageSize + 60*1024*1024
					limits.Disk.Scope = garden.DiskLimitScopeTotal
				} else {
					limits.Disk.ByteSoft = 20 * 1024 * 1024
					limits.Disk.ByteHard = 20 * 1024 * 1024
					limits.Disk.Scope = garden.DiskLimitScopeTotal
				}
			})

			It("reports initial total bytes of a container based on size of image", func() {
				metrics, err := container.Metrics()
				Expect(err).ToNot(HaveOccurred())

				Expect(metrics.DiskStat.TotalBytesUsed).To(BeNumerically(">", metrics.DiskStat.ExclusiveBytesUsed))
				if runtime.GOOS == "windows" {
					Expect(metrics.DiskStat.TotalBytesUsed).To(BeNumerically("~", limitsTestContainerImageSize+containerStartUsage, 20*1024*1024))

				} else {
					Expect(metrics.DiskStat.TotalBytesUsed).To(BeNumerically("~", 4*1024*1024, 1024*1024)) // base busybox is ~4 MB
				}
			})

			Context("and run a process that does not exceed the limit", func() {
				It("does not kill the process", func() {
					var spec garden.ProcessSpec
					if runtime.GOOS == "windows" {
						spec = garden.ProcessSpec{
							Path: "fsutil",
							Args: []string{"file", "createnew", "foo.txt", strconv.Itoa(5 * 1024 * 1024)},
						}
					} else {
						spec = garden.ProcessSpec{
							User: "root",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/root/test", "bs=1M", "count=7"},
						}
					}
					exitCode, _, _ := runProcess(container, spec)
					Expect(exitCode).To(Equal(0))
				})
			})

			Context("and run a process that exceeds the quota due to the size of the rootfs", func() {
				var fileTooBigSize int

				BeforeEach(func() {
					fileTooBigSize = 120 * 1024 * 1024
				})

				It("kills the process", func() {
					var spec garden.ProcessSpec
					if runtime.GOOS == "windows" {
						spec = garden.ProcessSpec{
							Path: "fsutil",
							Args: []string{"file", "createnew", "foo.txt", strconv.Itoa(fileTooBigSize)},
						}

					} else {
						spec = garden.ProcessSpec{
							User: "root",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/root/test", "bs=1M", "count=20"},
						}
					}
					exitCode, _, _ := runProcess(container, spec)
					Expect(exitCode).ToNot(Equal(0))
				})
			})

			Context("when rootfs exceeds the quota", func() {
				BeforeEach(func() {
					assertContainerCreate = false
					if runtime.GOOS == "windows" {
						limits.Disk.ByteHard = 1024 * 1024 * 1024
					} else {
						imageRef.URI = "docker:///ubuntu#trusty-20160323"
					}
				})

				It("should fail to create a container", func() {
					Expect(containerCreateErr).To(HaveOccurred())
				})
			})
		})

		Context("when the scope is exclusive", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					limits.Disk.ByteSoft = 60 * 1024 * 1024
					limits.Disk.ByteHard = 60 * 1024 * 1024
					limits.Disk.Scope = garden.DiskLimitScopeExclusive
				} else {
					limits.Disk.ByteSoft = 10 * 1024 * 1024
					limits.Disk.ByteHard = 10 * 1024 * 1024
					limits.Disk.Scope = garden.DiskLimitScopeExclusive
				}
			})

			Context("and run a process that would exceed the quota due to the size of the rootfs", func() {
				It("does not kill the process", func() {
					var spec garden.ProcessSpec
					if runtime.GOOS == "windows" {
						spec = garden.ProcessSpec{
							Path: "fsutil",
							Args: []string{"file", "createnew", "foo.txt", strconv.Itoa(15 * 1024 * 1024)},
						}

					} else {
						spec = garden.ProcessSpec{
							User: "root",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/root/test", "bs=1M", "count=9"}, // should succeed, even though equivalent with 'total' scope does not
						}
					}
					exitCode, _, _ := runProcess(container, spec)
					Expect(exitCode).To(Equal(0))
				})
			})

			Context("and run a process that exceeds the quota", func() {
				It("kills the process", func() {
					var spec garden.ProcessSpec
					if runtime.GOOS == "windows" {
						spec = garden.ProcessSpec{
							Path: "fsutil",
							Args: []string{"file", "createnew", "foo.txt", strconv.Itoa(55 * 1024 * 1024)},
						}
					} else {
						spec = garden.ProcessSpec{
							User: "root",
							Path: "dd",
							Args: []string{"if=/dev/zero", "of=/root/test", "bs=1M", "count=11"},
						}
					}
					exitCode, _, _ := runProcess(container, spec)
					Expect(exitCode).ToNot(Equal(0))
				})
			})
		})

		Context("a rootfs with pre-existing users", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				limits.Disk.ByteSoft = 10 * 1024 * 1024
				limits.Disk.ByteHard = 10 * 1024 * 1024
				limits.Disk.Scope = garden.DiskLimitScopeExclusive
			})

			JustBeforeEach(func() {
				createUser(container, "alice")
				createUser(container, "bob")
			})

			Context("and run a process that exceeds the quota as bob", func() {
				It("kills the process", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "bob",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/bob/test", "bs=1M", "count=11"},
					})
					Expect(exitCode).ToNot(Equal(0))
				})
			})

			Context("and run a process that exceeds the quota as alice", func() {
				It("kills the process", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/alice/test", "bs=1M", "count=11"},
					})
					Expect(exitCode).ToNot(Equal(0))
				})
			})

			Context("user alice is getting near the set limit", func() {
				JustBeforeEach(func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/alice/test", "bs=1M", "count=8"},
					})
					Expect(exitCode).To(Equal(0))
				})

				It("kills the process if user bob tries to exceed the shared limit", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "bob",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/bob/test", "bs=1M", "count=3"},
					})
					Expect(exitCode).ToNot(Equal(0))
				})
			})

			Context("when multiple containers are created for the same user", func() {
				var container2 garden.Container
				var err error

				BeforeEach(func() {
					limits.Disk.ByteSoft = 50 * 1024 * 1024
					limits.Disk.ByteHard = 50 * 1024 * 1024
					limits.Disk.Scope = garden.DiskLimitScopeExclusive
				})

				JustBeforeEach(func() {
					container2, err = gardenClient.Create(garden.ContainerSpec{
						Privileged: privilegedContainer,
						Image:      imageRef,
						Limits:     limits,
					})
					Expect(err).ToNot(HaveOccurred())

					createUser(container2, "alice")
				})

				AfterEach(func() {
					if container2 != nil {
						Expect(gardenClient.Destroy(container2.Handle())).To(Succeed())
					}
				})

				It("gives each container its own quota", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/urandom", "of=/tmp/some-file", "bs=1M", "count=40"},
					})
					Expect(exitCode).To(Equal(0))

					exitCode, _, _ = runProcess(container2, garden.ProcessSpec{
						User: "alice",
						Path: "dd",
						Args: []string{"if=/dev/urandom", "of=/tmp/some-file", "bs=1M", "count=40"},
					})
					Expect(exitCode).To(Equal(0))
				})
			})
		})

		Context("when the container is privileged", func() {
			BeforeEach(func() {
				if runtime.GOOS == "windows" {
					Skip("pending for windows")
				}
				setPrivileged()

				limits.Disk.ByteSoft = 10 * 1024 * 1024
				limits.Disk.ByteHard = 10 * 1024 * 1024
				limits.Disk.Scope = garden.DiskLimitScopeExclusive
			})

			Context("and run a process that exceeds the quota as root", func() {
				It("kills the process", func() {
					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "root",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/root/test", "bs=1M", "count=11"},
					})
					Expect(exitCode).ToNot(Equal(0))
				})
			})

			Context("and run a process that exceeds the quota as a new user", func() {
				It("kills the process", func() {
					createUser(container, "bob")

					exitCode, _, _ := runProcess(container, garden.ProcessSpec{
						User: "bob",
						Path: "dd",
						Args: []string{"if=/dev/zero", "of=/home/bob/test", "bs=1M", "count=11"},
					})
					Expect(exitCode).ToNot(Equal(0))
				})
			})
		})
	})

	Describe("PID limits", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
		})
		Context("when there is a pid limit applied", func() {
			BeforeEach(func() {
				major, minor := getKernelVersion()
				if major < 4 || (major == 4 && minor < 4) {
					Skip("kernel version should be at 4.4 or later")
				}

				limits.Pid = garden.PidLimits{Max: 50}
				imageRef.URI = limitsTestURI
			})

			It("prevents forking of processes", func() {
				exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", "for i in `seq 1 50`; do /bin/sleep 2 & done"},
				})

				Expect(exitCode).To(Equal(2))
				Expect(stderr).To(gbytes.Say(`sh: (?:line \d+: )?can't fork`))
			})
		})

		Context("when the pid limit is set to 0", func() {
			BeforeEach(func() {
				limits.Pid = garden.PidLimits{Max: 0}
			})

			It("applies no limit", func() {
				exitCode, _, _ := runProcess(container, garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", "ps"},
				})
				Expect(exitCode).To(Equal(0))
			})
		})
	})

	Describe("FD limits", func() {
		BeforeEach(func() {
			if runtime.GOOS == "windows" {
				Skip("pending for windows")
			}
			imageRef.URI = limitsTestURI
			imageRef.Username = "gfranks"
			imageRef.Password = "iXtJhLixuMrFWhgkarncpKRJjhTbbsakqgEVzzxoYb6HZWZFuRpyUUJ4wENACejU"
		})
		Context("When there is a FD limit applied", func() {
			var processSpec garden.ProcessSpec
			BeforeEach(func() {
				nofile := uint64(30)
				// this test runs as busybox, which has a `sh` binary capable of handling FDs >9
				processSpec = garden.ProcessSpec{
					User: "root",
					Path: "sh",
					Args: []string{"-c", `
set -e

# must start after fd 2
exec 10<>file1
exec 11<>file2
exec 12<>file3
exec 13<>file4
exec 14<>file5
exec 15<>file6
exec 16<>file7
exec 17<>file8
exec 18<>file9
exec 19<>file10
exec 20<>file11
exec 21<>file12
exec 22<>file13
exec 23<>file14
exec 24<>file15
exec 25<>file16
exec 26<>file17
exec 27<>file18
exec 28<>file19
exec 29<>file20
exec 30<>file21
exec 31<>file22
exec 32<>file23
exec 33<>file24
exec 34<>file25
exec 35<>file26
exec 36<>file27
exec 37<>file28
exec 38<>file29
exec 39<>file30
exec 41<>file31

echo should have died by now
`},
					Limits: garden.ResourceLimits{
						Nofile: &nofile,
					},
				}
			})

			It("applies the FD limit to containers", func() {
				exitCode, _, stderr := runProcess(container, processSpec)
				Expect(exitCode).To(Equal(1))
				Eventually(stderr, "1s").Should(gbytes.Say(`sh: (?:line \d+: )?dup2\(3,30\): Bad file descriptor`))
			})
		})
	})
})

type archiveFile struct {
	Name string
	Body []byte
	Mode int64
	Dir  bool
	Link string
}

func createTarGZArchive(filename string, files []archiveFile) {
	file, err := os.Create(filename)
	Expect(err).NotTo(HaveOccurred())

	gw := gzip.NewWriter(file)

	writeTar(gw, files)

	err = gw.Close()
	Expect(err).NotTo(HaveOccurred())

	err = file.Close()
	Expect(err).NotTo(HaveOccurred())
}

func writeTar(destination io.Writer, files []archiveFile) {
	w := tar.NewWriter(destination)

	for _, file := range files {
		var header *tar.Header

		mode := file.Mode
		if mode == 0 {
			mode = 0777
		}

		if file.Dir {
			header = &tar.Header{
				Name:     file.Name,
				Mode:     0755,
				Typeflag: tar.TypeDir,
			}
		} else if file.Link != "" {
			header = &tar.Header{
				Name:     file.Name,
				Typeflag: tar.TypeSymlink,
				Linkname: file.Link,
				Mode:     file.Mode,
			}
		} else {
			header = &tar.Header{
				Name: file.Name,
				Mode: mode,
				Size: int64(len(file.Body)),
			}
		}

		err := w.WriteHeader(header)
		Expect(err).NotTo(HaveOccurred())

		_, err = w.Write(file.Body)
		Expect(err).NotTo(HaveOccurred())
	}

	err := w.Close()
	Expect(err).NotTo(HaveOccurred())
}
