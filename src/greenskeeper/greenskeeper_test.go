package greenskeeper

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var _ = Describe("Greenskeeper", func() {
	Describe("#CheckExistingGdnProcess", func() {
		var (
			pidFile *os.File
		)

		BeforeEach(func() {
			var err error
			pidFile, err = ioutil.TempFile("", "pidfile")
			Expect(err).NotTo(HaveOccurred())
			pidFile.Close()
		})

		AfterEach(func() {
			os.Remove(pidFile.Name())
			Expect(pidFile.Name()).NotTo(BeAnExistingFile())
		})

		It("removes stale pidfiles", func() {
			Expect(CheckExistingGdnProcess(pidFile.Name())).To(Succeed())
			Expect(pidFile.Name()).NotTo(BeAnExistingFile())
		})

		Context("when removing the pidfile fails", func() {
			It("returns an error", func() {
				failingRemove := func(string) error { return errors.New("I failed") }
				Expect(checkExistingGdnProcess(pidFile.Name(), failingRemove)).NotTo(Succeed())
			})
		})

		Context("when the pidfile doesn't exist", func() {
			It("does not return an error", func() {
				Expect(CheckExistingGdnProcess("/does/not/exist")).To(Succeed())
			})
		})

		Context("when the PIDFILE refers to a running process", func() {
			var (
				sleepSession *gexec.Session
				sleepPid     string
			)

			BeforeEach(func() {
				sleepSession = gexecStart(exec.Command("sleep", "60"), GinkgoWriter, GinkgoWriter)
				sleepPid = strconv.Itoa(sleepSession.Command.Process.Pid)

				Expect(ioutil.WriteFile(pidFile.Name(), []byte(sleepPid), os.ModePerm)).To(Succeed())
			})

			AfterEach(func() {
				sleepSession.Kill().Wait()
			})

			It("returns an error", func() {
				err := CheckExistingGdnProcess(pidFile.Name())
				Expect(err).To(MatchError(fmt.Sprintf("garden is already running (pid: %s)", sleepPid)))
			})
		})
	})
})

func gexecStart(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	return session
}
