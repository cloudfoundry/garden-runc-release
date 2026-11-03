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
			envs = gardenEnvs(tmpDir, pidFileName)

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
			Entry("RUNTIME_BIN_DIR", "garden/bin", 0750, 0, maxID),
		)

		Context("rootfull directories", func() {
			DescribeTable("are created with root owner",
				func(dir string, mode, uid, gid int) {
					checkPermissionsAndOwnership(tmpDir, dir, mode, uid, gid)
				},
				Entry("LOG_DIR", "log", 0770, 0, 0),
				Entry("TMPDIR", "tmp", 0755, 0, 0),
				Entry("DEPOT_PATH", "garden/depot", 0755, 0, 0),
			)
		})
	})

	Context("when GARDEN_DATA_DIR is vcap-owned from a previous run", func() {
		var (
			tmpDir     string
			gardenDir  string
			victimDir  string
			envs       []string
			victimStat *syscall.Stat_t
		)

		BeforeEach(func() {
			var err error
			tmpDir, err = os.MkdirTemp("", "")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.Chmod(tmpDir, 0755)).To(Succeed())

			gardenDir = path.Join(tmpDir, "garden")
			Expect(os.Mkdir(gardenDir, 0770)).To(Succeed())
			Expect(os.Chown(gardenDir, vcapID, maxID)).To(Succeed())

			victimDir, err = os.MkdirTemp(tmpDir, "victim")
			Expect(err).ToNot(HaveOccurred())
			Expect(os.Chmod(victimDir, 0700)).To(Succeed())
			victimStat = stat(victimDir).Sys().(*syscall.Stat_t)

			envs = gardenEnvs(tmpDir, tempFile("pidfile", tmpDir))
			gkCmd = exec.Command(gkBin)
			gkCmd.Env = envs
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpDir)).To(Succeed())
		})

		Context("and vcap replaced the depot with a symlink to another directory", func() {
			BeforeEach(func() {
				Expect(os.Symlink(victimDir, path.Join(gardenDir, "depot"))).To(Succeed())
			})

			It("fails without touching the symlink target", func() {
				session := gexecStartAndWait(gkCmd, GinkgoWriter, GinkgoWriter)
				Expect(session.ExitCode()).NotTo(Equal(0))

				fileInfo := stat(victimDir)
				Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(0700)))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Uid).To(Equal(victimStat.Uid))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Gid).To(Equal(victimStat.Gid))
			})
		})

		Context("and vcap replaced the runtime bin dir with a symlink to another directory", func() {
			BeforeEach(func() {
				Expect(os.Symlink(victimDir, path.Join(gardenDir, "bin"))).To(Succeed())
			})

			It("fails without touching the symlink target", func() {
				session := gexecStartAndWait(gkCmd, GinkgoWriter, GinkgoWriter)
				Expect(session.ExitCode()).NotTo(Equal(0))

				fileInfo := stat(victimDir)
				Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(0700)))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Uid).To(Equal(victimStat.Uid))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Gid).To(Equal(victimStat.Gid))
			})
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

// gardenEnvs mirrors the production layout, where DEPOT_PATH and
// RUNTIME_BIN_DIR are children of GARDEN_DATA_DIR.
func gardenEnvs(tmpDir, pidFileName string) []string {
	gardenDir := path.Join(tmpDir, "garden")
	return []string{
		"PIDFILE=" + pidFileName,
		"RUN_DIR=" + path.Join(tmpDir, "run"),
		"GARDEN_DATA_DIR=" + gardenDir,
		"LOG_DIR=" + path.Join(tmpDir, "log"),
		"TMPDIR=" + path.Join(tmpDir, "tmp"),
		"DEPOT_PATH=" + path.Join(gardenDir, "depot"),
		"RUNTIME_BIN_DIR=" + path.Join(gardenDir, "bin"),
		"MAXIMUS=" + strconv.Itoa(maxID),
	}
}

func stat(p string) os.FileInfo {
	fileInfo, err := os.Stat(p)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return fileInfo
}

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
