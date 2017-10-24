package greenskeeper_integration_test

import (
	"io"
	"os"
	"os/exec"
	"path"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Greenskeeper", func() {
	var (
		envs        []string
		pidFileName string
		stdout      *gbytes.Buffer
		gkCmd       *exec.Cmd
		session     *gexec.Session
	)

	BeforeEach(func() {
		pidFileName = tempFile("pidfile")
		envs = []string{
			"PIDFILE=" + pidFileName,
			"RUN_DIR=" + path.Join(tmpDir, "run"),
			"GARDEN_DIR=" + path.Join(tmpDir, "garden"),
			"LOG_DIR=" + path.Join(tmpDir, "log"),
			"TMPDIR=" + path.Join(tmpDir, "tmp"),
			"DEPOT_PATH=" + path.Join(tmpDir, "depot"),
			"RUNTIME_BIN_DIR=" + path.Join(tmpDir, "bin"),
			"GRAPH_PATH=" + path.Join(tmpDir, "graph"),
		}

		gkCmd = exec.Command(gkBin)
		gkCmd.Env = envs
		stdout = gbytes.NewBuffer()
	})

	JustBeforeEach(func() {
		session = gexecStartAndWait(gkCmd, stdout, GinkgoWriter)
	})

	It("prepares the system for running gdn", func() {
		Expect(session.ExitCode()).To(Equal(0))

		By("removing stale pidfiles")
		Expect(pidFileName).NotTo(BeAnExistingFile())
		Expect(stdout).To(gbytes.Say("Removing stale pidfile..."))

		By("creating and chowning all the required directories")
		expectDirectory(path.Join(tmpDir, "run"), 0755)
		expectDirectory(path.Join(tmpDir, "garden"), 0770)
		expectDirectory(path.Join(tmpDir, "log"), 0755)
		expectDirectory(path.Join(tmpDir, "tmp"), 0755)
		expectDirectory(path.Join(tmpDir, "depot"), 0755)
		expectDirectory(path.Join(tmpDir, "bin"), 0750)
		expectDirectory(path.Join(tmpDir, "graph"), 0700)
	})

	Context("when an env isn't set", func() {
		BeforeEach(func() {
			gkCmd.Env = []string{}
		})

		It("exits with failure", func() {
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

func tempFile(name string) string {
	file, err := os.Create(path.Join(tmpDir, name))
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
