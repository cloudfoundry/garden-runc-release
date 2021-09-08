package main_test

import (
	"io"
	"os/exec"
	"syscall"
	"testing"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

var (
	thresholderBin string
	diskSize       int64
)

const (
	fsFile       = "./fsFile"
	fsMountPoint = "./mnt"
	fsSize       = "20G"
)

func TestThresholder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Thresholder Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	thresholderBin, err = gexec.Build("thresholder", "-mod=vendor")
	Expect(err).ToNot(HaveOccurred())

	createAndMountFilesystem(fsFile, fsSize, fsMountPoint)
	diskSize = getDiskAvailableSpace(fsMountPoint)
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
	cleanupFilesystem(fsFile, fsMountPoint)
})

func gexecStartAndWait(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session).Should(gexec.Exit())
	return session
}

func getDiskAvailableSpace(diskPath string) int64 {
	var fsStat syscall.Statfs_t
	Expect(syscall.Statfs(diskPath, &fsStat)).To(Succeed())
	return fsStat.Bsize * int64(fsStat.Bavail)
}

func createAndMountFilesystem(filename, size, mntPoint string) {
	err := exec.Command("truncate", "-s", size, filename).Run()
	Expect(err).NotTo(HaveOccurred())

	err = exec.Command("mkfs.ext4", filename).Run()
	Expect(err).NotTo(HaveOccurred())

	err = exec.Command("mkdir", mntPoint).Run()
	Expect(err).NotTo(HaveOccurred())

	err = exec.Command("mount", filename, mntPoint).Run()
	Expect(err).NotTo(HaveOccurred())
}

func cleanupFilesystem(filename, mntPoint string) {
	err := exec.Command("umount", mntPoint).Run()
	Expect(err).NotTo(HaveOccurred())

	err = exec.Command("rm", "-rf", filename, mntPoint).Run()
	Expect(err).NotTo(HaveOccurred())
}
