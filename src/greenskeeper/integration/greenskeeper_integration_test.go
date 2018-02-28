package greenskeeper_integration_test

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
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
			tmpDir, err = ioutil.TempDir("", "")
			Expect(err).ToNot(HaveOccurred())
			pidFileName = tempFile("pidfile", tmpDir)
			envs = []string{
				"PIDFILE=" + pidFileName,
				"RUN_DIR=" + path.Join(tmpDir, "run"),
				"GARDEN_DIR=" + path.Join(tmpDir, "garden"),
				"LOG_DIR=" + path.Join(tmpDir, "log"),
				"TMPDIR=" + path.Join(tmpDir, "tmp"),
				"DEPOT_PATH=" + path.Join(tmpDir, "depot"),
				"RUNTIME_BIN_DIR=" + path.Join(tmpDir, "bin"),
				"GRAPH_PATH=" + path.Join(tmpDir, "graph"),
				"MAXIMUS=4294967294",
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
				Expect(path.Join(tmpDir, dir)).To(BeADirectory())
				fileInfo, err := os.Stat(path.Join(tmpDir, dir))
				Expect(err).NotTo(HaveOccurred())
				Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(mode)))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Uid).To(Equal(uint32(uid)))
				Expect(fileInfo.Sys().(*syscall.Stat_t).Gid).To(Equal(uint32(gid)))
			},
			Entry("RUN_DIR", "run", 0770, 0, 0),
			Entry("GARDEN_DIR", "garden", 0770, vcapID, 4294967294),
			Entry("LOG_DIR", "log", 0770, 0, 0),
			Entry("TMPDIR", "tmp", 0755, 0, 0),
			Entry("DEPOT_PATH", "depot", 0755, 0, 0),
			Entry("RUNTIME_BIN_DIR", "bin", 0750, 0, 4294967294),
			Entry("GRAPH_PATH", "graph", 0700, 4294967294, 4294967294),
		)

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

func expectDirectory(path string, mode os.FileMode) {
	ExpectWithOffset(1, path).To(BeADirectory())
	fileInfo, err := os.Stat(path)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	ExpectWithOffset(1, fileInfo.Mode().Perm()).To(Equal(mode))
}
