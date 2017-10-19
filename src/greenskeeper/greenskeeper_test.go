package greenskeeper

import (
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"os/user"
	"path"
	"strconv"
	"syscall"

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

	Describe("#SetupDirectories", func() {
		var (
			dir        string
			pikachuDir string
			setupErr   error
			directory  Directory
		)

		BeforeEach(func() {
			var err error
			dir, err = ioutil.TempDir("", "")
			Expect(err).NotTo(HaveOccurred())
			pikachuDir = path.Join(dir, "pikachu")
			directory = NewDirectory(pikachuDir, 0644, "mew", "mewtwo")
		})

		JustBeforeEach(func() {
			setupErr = SetupDirectories(directory)
		})

		AfterEach(func() {
			Expect(os.RemoveAll(dir)).To(Succeed())
		})

		It("does not return an error", func() {
			Expect(setupErr).NotTo(HaveOccurred())
		})

		It("sets up a directory", func() {
			By("creating the directory")
			Expect(pikachuDir).To(BeAnExistingFile())

			By("setting the correct permissions")
			fileInfo, err := os.Stat(pikachuDir)
			Expect(err).NotTo(HaveOccurred())
			Expect(fileInfo.Mode().Perm()).To(Equal(os.FileMode(0644)))

			By("setting the correct user ownership")
			mewUser, err := user.Lookup("mew")
			Expect(err).NotTo(HaveOccurred())
			mewUID := atouint32(mewUser.Uid)
			Expect(fileInfo.Sys().(*syscall.Stat_t).Uid).To(Equal(mewUID))

			By("setting the correct group ownership")
			mewGID := atouint32(mewUser.Gid)
			Expect(fileInfo.Sys().(*syscall.Stat_t).Gid).To(Equal(mewGID))
		})

		Context("when creating a directory fails", func() {
			BeforeEach(func() {
				directory.mkdirAll = func(string, os.FileMode) error { return errors.New("I failed") }
			})

			It("returns an error", func() {
				Expect(setupErr).To(HaveOccurred())
			})
		})

		Context("when the user does not exist", func() {
			BeforeEach(func() {
				directory.User = "missingno"
			})

			It("returns an error", func() {
				Expect(setupErr).To(HaveOccurred())
			})
		})
	})
})

func gexecStart(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	return session
}

func atouint32(n string) uint32 {
	i, err := strconv.Atoi(n)
	Expect(err).NotTo(HaveOccurred())
	return uint32(i)
}
