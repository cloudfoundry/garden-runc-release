package greenskeeper_integration_test

import (
	"io"
	"os"
	"os/exec"
	"path"
	"strconv"
	"syscall"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Greenskeeper", func() {
	var (
		gkCmd *exec.Cmd
	)

	Context("happy path", func() {
		var (
			envs        []string
			pidFileName string
			tmpDir      string
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())
			pidFileName = tempFile("pidfile", tmpDir)
			envs = []string{
				"PIDFILE=" + pidFileName,
				"RUN_DIR=" + path.Join(tmpDir, "run"),
				"GARDEN_DATA_DIR=" + path.Join(tmpDir, "garden"),
				"CONTAINERD_DATA_DIR=" + path.Join(tmpDir, "containerd"),
				"LOG_DIR=" + path.Join(tmpDir, "log"),
				"TMPDIR=" + path.Join(tmpDir, "tmp"),
				"DEPOT_PATH=" + path.Join(tmpDir, "depot"),
				"RUNTIME_BIN_DIR=" + path.Join(tmpDir, "bin"),
				"XDG_RUNTIME_DIR=" + path.Join(tmpDir, "xdg"),
				"GARDEN_ROOTLESS_CONFIG_DIR=" + path.Join(tmpDir, "rootless-config"),
				"MAXIMUS=" + strconv.Itoa(maxID),
			}

			gkCmd = exec.Command(gkBin)
			gkCmd.Env = envs
		})

		JustBeforeEach(func() {
			session := gexecStartAndWait(gkCmd, GinkgoWriter, GinkgoWriter)
			Expect(session.ExitCode()).To(Equal(0))
			Expect(pidFileName).NotTo(BeAnExistingFile())
			Expect(session.Out).To(gbytes.Say("Removing stale pidfile..."))
		})

		AfterEach(func() {
			os.RemoveAll(tmpDir)
			Expect(tmpDir).NotTo(BeADirectory())
		})

		DescribeTable("prepares the directories for the gdn",
			func(dir string, mode, uid, gid int) {
				checkPermissionsAndOwnership(tmpDir, dir, mode, uid, gid)
			},
			Entry("RUN_DIR", "run", 0770, 0, 0),
			Entry("GARDEN_DATA_DIR", "garden", 0770, vcapID, maxID),
			Entry("RUNTIME_BIN_DIR", "bin", 0750, 0, maxID),
		)

		Context("rootfull directories", func() {
			DescribeTable("are created with root owner",
				func(dir string, mode, uid, gid int) {
					checkPermissionsAndOwnership(tmpDir, dir, mode, uid, gid)
				},
				Entry("LOG_DIR", "log", 0770, 0, 0),
				Entry("TMPDIR", "tmp", 0755, 0, 0),
				Entry("DEPOT_PATH", "depot", 0755, 0, 0),
			)
		})

		Context("rootless directories", func() {
			BeforeEach(func() {
				gkCmd = exec.Command(gkBin, "--rootless")
				gkCmd.Env = envs
			})

			DescribeTable("are created with max owner",
				func(dir string, mode, uid, gid int) {
					checkPermissionsAndOwnership(tmpDir, dir, mode, uid, gid)
				},
				Entry("LOG_DIR", "log", 0770, maxID, maxID),
				Entry("TMPDIR", "tmp", 0755, maxID, maxID),
				Entry("DEPOT_PATH", "depot", 0755, maxID, maxID),
				Entry("XDG_RUNTIME_DIR", "xdg", 0700, maxID, maxID),
				Entry("CONTAINERD_DATA_DIR", "containerd", 0700, maxID, maxID),
				Entry("GARDEN_ROOTLESS_CONFIG_DIR", "rootless-config", 0700, maxID, maxID),
			)
		})
	})

	Context("when an env isn't set", func() {
		BeforeEach(func() {
			gkCmd = exec.Command(gkBin)
			gkCmd.Env = []string{}
		})

		It("exits with failure", func() {
			session := gexecStartAndWait(gkCmd, nil, GinkgoWriter)
			Expect(session.ExitCode()).To(Equal(1))
		})
	})
})

func checkPermissionsAndOwnership(tmpDir, dir string, mode, uid, gid int) {
	Expect(path.Join(tmpDir, dir)).To(BeADirectory())
	fileInfo, err := os.Stat(path.Join(tmpDir, dir))
	Expect(err).NotTo(HaveOccurred())
	Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(mode)))
	Expect(fileInfo.Sys().(*syscall.Stat_t).Uid).To(Equal(uint32(uid)))
	Expect(fileInfo.Sys().(*syscall.Stat_t).Gid).To(Equal(uint32(gid)))
}

func gexecStartAndWait(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session).Should(gexec.Exit())
	return session
}

func tempFile(name, tmp string) string {
	file, err := os.Create(path.Join(tmp, name))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	file.Close()
	return file.Name()
}
