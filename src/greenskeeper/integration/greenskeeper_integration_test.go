package greenskeeper_integration_test

import (
	"io"
	"io/ioutil"
	"os"
	"os/exec"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Greenskeeper", func() {
	It("prepares the system for running gdn", func() {
		By("removing stale pidfiles")
		pidFile := tempFile("", "pidfile")
		pidFile.Close()
		gkCmd := exec.Command(gkBin)
		gkCmd.Env = append(gkCmd.Env, "PIDFILE="+pidFile.Name())
		stdout := gbytes.NewBuffer()
		gexecStartAndWait(gkCmd, stdout, GinkgoWriter)
		Expect(pidFile.Name()).NotTo(BeAnExistingFile())
		Expect(stdout).To(gbytes.Say("Removing stale pidfile..."))
	})
})

func gexecStartAndWait(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session).Should(gexec.Exit(0))
	return session
}

func tempFile(dir, prefix string) *os.File {
	file, err := ioutil.TempFile(dir, prefix)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return file
}
