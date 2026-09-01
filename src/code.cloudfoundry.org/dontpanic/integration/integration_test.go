package integration_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/unix"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

const (
	gardenConfigDir      = "/var/vcap/jobs/garden/config"
	gardenLogDir         = "/var/vcap/sys/log/garden"
	varLogDir            = "/var/log"
	gardenDepotDir       = "/var/vcap/data/garden/depot"
	monitDir             = "/var/vcap/monit"
	containerdSocketPath = "/var/vcap/sys/run/containerd/containerd.sock"
	dateRegexp           = `\w{3} \w{3}  ?\d{1,2}.*\d{4}.*`
	iniFileTemplate      = `
[server]
  log-level = %s`
)

var _ = Describe("Integration", func() {
	var (
		bindMounts []string = []string{"/proc", "/sys", "/dev", "/bin", "/usr/bin", "/sbin", "/usr/sbin", "/lib", "/lib64"}
		sandboxDir string
		session    *gexec.Session
		cmd        *exec.Cmd
	)

	BeforeEach(func() {
		var err error
		sandboxDir, err = os.MkdirTemp("", "dontpanic-sandbox")
		Expect(err).NotTo(HaveOccurred())
		createTestResources(sandboxDir)
		mountAll(sandboxDir, bindMounts)
		cmd = exec.Command("chroot", sandboxDir, "./dontpanic")
	})

	AfterEach(func() {
		unmountAll(sandboxDir, bindMounts)
		Expect(os.RemoveAll(gardenConfigDir)).To(Succeed())
		Expect(os.RemoveAll(gardenLogDir)).To(Succeed())
	})

	JustBeforeEach(func() {
		var err error
		session, err = gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
		Expect(err).NotTo(HaveOccurred())
		Eventually(session, time.Second*70).Should(gexec.Exit())
	})

	It("produces a report correctly", func() {
		reportDir := filepath.Join(sandboxDir, getReportDir(session.Out.Contents()))
		tarPath := reportDir + ".tar.gz"

		By("succeeding")
		Expect(session.ExitCode()).To(Equal(0))

		By("not showing the debug level warning message")
		Expect(session).NotTo(gbytes.Say("WARNING: Garden log level is set to error. This usually makes problem diagnosis quite hard."))

		By("showing an initial message")
		Expect(session).To(gbytes.Say("<Useful information below, please copy-paste from here>"))

		By("writing a logfile containing all steps")
		tarballShouldContainFile(tarPath, "dontpanic.log")
		Expect(string(tarballFileContents(tarPath, "dontpanic.log"))).
			To(ContainSubstring("## Date"))

		By("collecting the date")
		tarballShouldContainFile(tarPath, "date.log")
		Expect(string(tarballFileContents(tarPath, "date.log"))).
			To(MatchRegexp(dateRegexp))

		By("collecting the uptime")
		tarballShouldContainFile(tarPath, "uptime.log")
		Expect(string(tarballFileContents(tarPath, "uptime.log"))).
			To(ContainSubstring("load average"))

		By("collecting the garden version")
		Expect(session).To(gbytes.Say("## Garden Version"))

		By("collecting the hostname")
		tarballShouldContainFile(tarPath, "hostname.log")

		By("collecting the free memory")
		tarballShouldContainFile(tarPath, "free.log")
		Expect(string(tarballFileContents(tarPath, "free.log"))).
			To(ContainSubstring("Mem:"))

		By("collecting the kernel details")
		tarballShouldContainFile(tarPath, "uname.log")
		Expect(string(tarballFileContents(tarPath, "uname.log"))).
			To(ContainSubstring("Linux"))

		By("collecting monit summary")
		Expect(session).To(gbytes.Say("## Monit Summary"))

		By("collecting the number of open files")
		tarballShouldContainFile(tarPath, "num-open-files.log")
		Expect(tarballFileContents(tarPath, "num-open-files.log")).ToNot(BeEmpty())

		By("collecting the max number of open files")
		tarballShouldContainFile(tarPath, "file-max.log")
		Expect(tarballFileContents(tarPath, "file-max.log")).ToNot(BeEmpty())

		By("collecting the disk usage")
		tarballShouldContainFile(tarPath, "df.log")
		Expect(tarballFileContents(tarPath, "df.log")).To(ContainSubstring("Filesystem"))

		By("collecting the open files")
		tarballShouldContainFile(tarPath, "lsof.log")
		Expect(tarballFileContents(tarPath, "lsof.log")).To(ContainSubstring("COMMAND"))

		By("collecting the inodes")
		Expect(session).To(gbytes.Say("## Map of Inodes to Paths"))
		tarballShouldContainFile(tarPath, "inodes.log")
		Expect(tarballFileContents(tarPath, "inodes.log")).To(ContainSubstring("1 /proc"))

		By("collecting the process information")
		tarballShouldContainFile(tarPath, "ps-info.log")
		Expect(tarballFileContents(tarPath, "ps-info.log")).To(ContainSubstring("PID"))

		By("collecting the process forest information")
		tarballShouldContainFile(tarPath, "ps-forest.log")
		Expect(tarballFileContents(tarPath, "ps-forest.log")).To(ContainSubstring("USER"))

		By("collecting the dmesg")
		tarballShouldContainFile(tarPath, "dmesg.log")
		Expect(tarballFileContents(tarPath, "dmesg.log")).
			To(MatchRegexp(dateRegexp))

		By("collecting the network interfaces")
		tarballShouldContainFile(tarPath, "ifconfig.log")
		Expect(tarballFileContents(tarPath, "ifconfig.log")).To(ContainSubstring("Link"))

		By("collecting the firewall configuration")
		tarballShouldContainFile(tarPath, "iptables-L.log")
		Expect(tarballFileContents(tarPath, "iptables-L.log")).To(ContainSubstring("Chain"))

		By("collecting the NAT info")
		tarballShouldContainFile(tarPath, "iptables-tnat.log")
		Expect(tarballFileContents(tarPath, "iptables-tnat.log")).To(ContainSubstring("Chain"))

		By("collecting the mount table")
		Expect(session).To(gbytes.Say("## Mount Table"))

		By("collecting Garden depot contents")
		tarballShouldContainFile(tarPath, "depot-contents.log")
		Expect(tarballFileContents(tarPath, "depot-contents.log")).To(ContainSubstring("depot"))

		By("collecting XFS fragmentation info")
		Expect(session).To(gbytes.Say("## XFS Fragmentation"))

		By("collecting XFS info")
		Expect(session).To(gbytes.Say("## XFS Info"))

		By("collecting Slabinfo")
		tarballShouldContainFile(tarPath, "slabinfo.log")
		Expect(tarballFileContents(tarPath, "slabinfo.log")).To(ContainSubstring("active_objs"))

		By("collecting Meminfo")
		tarballShouldContainFile(tarPath, "meminfo.log")
		Expect(tarballFileContents(tarPath, "meminfo.log")).To(ContainSubstring("MemTotal"))

		By("collecting iostat")
		tarballShouldContainFile(tarPath, "iostat.log")
		Expect(tarballFileContents(tarPath, "iostat.log")).To(ContainSubstring("Linux"))

		By("collecting vm statistics")
		tarballShouldContainFile(tarPath, "vmstat-s.log")
		Expect(tarballFileContents(tarPath, "vmstat-s.log")).To(ContainSubstring("memory"))

		By("collecting disk statistics")
		tarballShouldContainFile(tarPath, "vmstat-d.log")
		Expect(tarballFileContents(tarPath, "vmstat-d.log")).To(ContainSubstring("disk"))

		By("collecting active and inactive memory statistics")
		tarballShouldContainFile(tarPath, "vmstat-a.log")
		Expect(tarballFileContents(tarPath, "vmstat-a.log")).To(ContainSubstring("memory"))

		By("collecting mass process data")
		currentPid := os.Getpid()
		tarballShouldContainFile(tarPath, filepath.Join("process-data", strconv.Itoa(currentPid), "fd"))
		tarballShouldContainFile(tarPath, filepath.Join("process-data", strconv.Itoa(currentPid), "ns"))
		tarballShouldContainFile(tarPath, filepath.Join("process-data", strconv.Itoa(currentPid), "cgroup"))
		tarballShouldContainFile(tarPath, filepath.Join("process-data", strconv.Itoa(currentPid), "stack"))
		tarballShouldContainFile(tarPath, filepath.Join("process-data", strconv.Itoa(currentPid), "status"))

		By("collecting the kernel logs")
		tarballShouldContainFile(tarPath, "kernel-logs/kern.log")
		tarballShouldContainFile(tarPath, "kernel-logs/kern.log.1")
		tarballShouldContainFile(tarPath, "kernel-logs/kern.log.2.gz")

		By("collecting monit log")
		tarballShouldContainFile(tarPath, "monit.log")
		Expect(tarballFileContents(tarPath, "monit.log")).To(ContainSubstring("monit"))

		By("collecting the syslogs")
		tarballShouldContainFile(tarPath, "syslogs/syslog")
		tarballShouldContainFile(tarPath, "syslogs/syslog.1")
		tarballShouldContainFile(tarPath, "syslogs/syslog.2.gz")

		By("collecting all garden config")
		tarballShouldContainFile(tarPath, "config/config.ini")
		Expect(string(tarballFileContents(tarPath, "config/config.ini"))).To(ContainSubstring("debug"))
		tarballShouldContainFile(tarPath, "config/grootfs_config.yml")
		tarballShouldContainFile(tarPath, "config/privileged_grootfs_config.yml")
		tarballShouldContainFile(tarPath, "config/bpm.yml")
		tarballShouldContainFile(tarPath, "config/containerd.toml")

		By("collecting the garden logs")
		tarballShouldContainFile(tarPath, "garden/garden.log")
		tarballShouldContainFile(tarPath, "garden/garden.log.1")
		tarballShouldContainFile(tarPath, "garden/garden.log.2.gz")

		By("collecting grootfs filesystem usage info")
		tarballShouldContainFile(tarPath, "grootfs/unprivileged-usage.txt")
		tarballShouldContainFile(tarPath, "grootfs/privileged-usage.txt")

		By("collecting systat")
		tarballShouldContainFile(tarPath, "sysstat/sa17")

		By("collecting the garden containers")
		Expect(session).To(gbytes.Say("## Garden Containers"))

		By("collecting the containerd init containers")
		Expect(session).To(gbytes.Say("## Containerd Init Containers"))

		By("collecting the containerd pea containers")
		Expect(session).To(gbytes.Say("## Containerd Pea Containers"))

		By("collecting the containerd tasks")
		Expect(session).To(gbytes.Say("## Containerd Tasks"))

		By("deleting the report dir at the end")
		Expect(reportDir).ToNot(BeADirectory())
	})

	When("running as a non-root user", func() {
		BeforeEach(func() {
			// we are not allowed to chroot when rootless, so exec out of sandbox
			cmd = exec.Command(dontPanicBin)
			cmd.SysProcAttr = &syscall.SysProcAttr{
				Credential: &syscall.Credential{Uid: 5000, Gid: 5000},
			}
		})

		It("warns and exits", func() {
			Expect(session.ExitCode()).ToNot(Equal(0))
			Expect(session.Err).To(gbytes.Say("Keep Calm and Re-run as Root!"))
		})
	})

	When("the garden log level is set to error", func() {
		BeforeEach(func() {
			newConfig := []byte(fmt.Sprintf(iniFileTemplate, "error"))
			Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "config.ini"), newConfig, 0644)).To(Succeed())
		})

		It("warns and continues", func() {
			Expect(session.ExitCode()).To(Equal(0))
			Expect(session.Err).To(gbytes.Say("WARNING: Garden log level is set to error. This usually makes problem diagnosis quite hard."))
		})
	})

	When("the garden log level is set to fatal", func() {
		BeforeEach(func() {
			newConfig := []byte(fmt.Sprintf(iniFileTemplate, "fatal"))
			Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "config.ini"), newConfig, 0644)).To(Succeed())
		})

		It("warns and continues", func() {
			Expect(session.ExitCode()).To(Equal(0))
			Expect(session.Err).To(gbytes.Say("WARNING: Garden log level is set to fatal. This usually makes problem diagnosis quite hard."))
		})
	})

	When("running in BPM", func() {
		BeforeEach(func() {
			// unmount proc (that's how dontpanic determines if it is inside bpm)
			Expect(unix.Unmount(filepath.Join(sandboxDir, "proc"), 0)).To(Succeed())
		})

		It("warns and exits", func() {
			Expect(session.ExitCode()).ToNot(Equal(0))
			Expect(session.Err).To(gbytes.Say("Cannot determine if running in bpm: cannot read cmdline"))
		})
	})

	When("running as non-containerd", func() {
		BeforeEach(func() {
			os.RemoveAll(filepath.Join(sandboxDir, containerdSocketPath))
		})

		It("should not collect containerd specific data", func() {
			By("collecting the containerd init containers")
			Expect(session).NotTo(gbytes.Say("## Containerd init containers"))

			By("collecting the containerd pea containers")
			Expect(session).NotTo(gbytes.Say("## Containerd pea containers"))

			By("collecting the containerd tasks")
			Expect(session).NotTo(gbytes.Say("## Containerd tasks"))
		})
	})

	When("passed the --sigquit flag", func() {
		var gdnSession *gexec.Session

		BeforeEach(func() {
			cmd.Args = append(cmd.Args, "--sigquit")
			gdnCmd := exec.Command("./assets/gdn")
			var err error
			gdnSession, err = gexec.Start(gdnCmd, GinkgoWriter, GinkgoWriter)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			gdnSession.Kill()
		})

		It("sends a SIGQUIT to the gdn process", func() {
			Eventually(gdnSession).Should(gexec.Exit(131))
		})
	})

	When("passed the --help flag", func() {
		BeforeEach(func() {
			cmd.Args = append(cmd.Args, "--help")
		})

		It("prints the usage and exits", func() {
			Expect(session.ExitCode()).To(Equal(0))
			Expect(session).To(gbytes.Say("Usage"))
			Expect(filepath.Join(sandboxDir, "var/vcap/data/tmp/")).NotTo(BeADirectory())
		})
	})

	When("passed an unknown flag", func() {
		BeforeEach(func() {
			cmd.Args = append(cmd.Args, "--hello")
		})

		It("prints the usage and exits", func() {
			Expect(session.ExitCode()).To(Equal(1))
			Expect(session.Err).To(gbytes.Say("unknown flag"))
			Expect(filepath.Join(sandboxDir, "var/vcap/data/tmp/")).NotTo(BeADirectory())
		})
	})
})

func tarballShouldContainFile(tarballPath, filePath string) {
	ExpectWithOffset(1, tarballPath).ToNot(BeEmpty(), "tarball not found: "+tarballPath)

	extractedOsReportPath := strings.TrimSuffix(filepath.Base(tarballPath), ".tar.gz")
	logFilePath := filepath.Join(extractedOsReportPath, filePath)
	ExpectWithOffset(1, listTarball(tarballPath)).To(ContainSubstring(logFilePath))
}

func tarballFileContents(tarballPath, filePath string) []byte {
	ExpectWithOffset(1, tarballPath).ToNot(BeEmpty(), "tarball not found: "+tarballPath)

	extractedOsReportPath := strings.TrimSuffix(filepath.Base(tarballPath), ".tar.gz")
	osDir := filepath.Base(extractedOsReportPath)

	cmd := exec.Command("tar", "xf", tarballPath, filepath.Join(osDir, filePath), "-O")
	out, err := cmd.Output()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return out
}

func listTarball(tarball string) string {
	cmd := exec.Command("tar", "tf", tarball)
	files, err := cmd.Output()
	ExpectWithOffset(2, err).NotTo(HaveOccurred())
	return string(files)
}

func getReportDir(output []byte) string {
	re := regexp.MustCompile(`(\/.*os-report-.*)\.tar\.gz`)
	matches := re.FindStringSubmatch(string(output))
	Expect(matches).To(HaveLen(2))
	return matches[1]
}

func mountAll(rootDir string, dirs []string) {
	for _, dir := range dirs {
		mountPoint := filepath.Join(rootDir, dir)
		Expect(os.MkdirAll(mountPoint, 0755)).To(Succeed())
		Expect(unix.Mount(dir, mountPoint, "", unix.MS_BIND, "")).To(Succeed())
	}
}

func unmountAll(rootDir string, dirs []string) {
	for _, dir := range dirs {
		unmount(filepath.Join(rootDir, dir))
	}
}

func unmount(mountPoint string) {
	err := unix.Unmount(mountPoint, 0)
	if err == unix.EINVAL {
		return
	}
	Expect(err).NotTo(HaveOccurred())
}

func createTestResources(sandboxDir string) {
	Expect(os.MkdirAll(filepath.Join(sandboxDir, gardenConfigDir), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "config.ini"), []byte(fmt.Sprintf(iniFileTemplate, "debug")), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "grootfs_config.yml"), []byte("store: /var/vcap/data/grootfs/store/unprivileged"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "privileged_grootfs_config.yml"), []byte("store: /var/vcap/data/grootfs/store/privileged"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "bpm.yml"), []byte("bpm"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenConfigDir, "containerd.toml"), []byte("nerd"), 0644)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, gardenLogDir), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenLogDir, "garden.log"), []byte("cur"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenLogDir, "garden.log.1"), []byte("prev"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, gardenLogDir, "garden.log.2.gz"), []byte("Z"), 0644)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, varLogDir), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "kern.log"), []byte("cur"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "kern.log.1"), []byte("prev"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "kern.log.2.gz"), []byte("Z"), 0644)).To(Succeed())

	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "syslog"), []byte("cur"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "syslog.1"), []byte("prev"), 0644)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "syslog.2.gz"), []byte("Z"), 0644)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, varLogDir, "sysstat"), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, varLogDir, "sysstat", "sa17"), []byte("meh"), 0644)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, monitDir), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, monitDir, "monit.log"), []byte("monit"), 0644)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, gardenDepotDir, "container1"), 0755)).To(Succeed())

	Expect(os.MkdirAll(filepath.Join(sandboxDir, filepath.Dir(containerdSocketPath)), 0755)).To(Succeed())
	Expect(os.WriteFile(filepath.Join(sandboxDir, containerdSocketPath), []byte("just-here"), 0644)).To(Succeed())

	Expect(exec.Command("cp", dontPanicBin, filepath.Join(sandboxDir, "dontpanic")).Run()).To(Succeed())
}
