package main_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os/exec"
	"strings"
	"syscall"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	thresholderBin string
	diskMountPath  string
	diskSize       int64
	loopDevice     string
)

func TestThresholder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Thresholder Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	thresholderBin, err = gexec.Build("thresholder")
	Expect(err).ToNot(HaveOccurred())

	diskFile, err := ioutil.TempFile("", "disk-file")
	Expect(err).NotTo(HaveOccurred())
	Expect(diskFile.Truncate(10 * 1024 * 1024)).To(Succeed())

	losetupOutput := bytes.NewBuffer([]byte{})
	losetupCmd := exec.Command("losetup", "--show", "-f", diskFile.Name())
	losetupCmd.Stderr = GinkgoWriter
	losetupCmd.Stdout = losetupOutput
	Expect(losetupCmd.Run()).To(Succeed())
	loopDevice = strings.TrimSuffix(losetupOutput.String(), "\n")

	diskMountPath, err = ioutil.TempDir("", "disk-path")
	Expect(err).NotTo(HaveOccurred())

	mountLoopCmd := exec.Command("mount", "-t", "tmpfs", loopDevice, diskMountPath)
	mountLoopCmd.Stdout = GinkgoWriter
	mountLoopCmd.Stderr = GinkgoWriter
	Expect(mountLoopCmd.Run()).To(Succeed())

	diskSize = getDiskSize(diskMountPath)
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()

	umountCmd := exec.Command("umount", diskMountPath)
	umountCmd.Stdout = GinkgoWriter
	umountCmd.Stderr = GinkgoWriter
	Expect(umountCmd.Run()).To(Succeed())

	losetupCmd := exec.Command("losetup", "-d", loopDevice)
	losetupCmd.Stderr = GinkgoWriter
	losetupCmd.Stdout = GinkgoWriter
	Expect(losetupCmd.Run()).To(Succeed())
})

func gexecStartAndWait(cmd *exec.Cmd, stdout, stderr io.Writer) *gexec.Session {
	session, err := gexec.Start(cmd, stdout, stderr)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session).Should(gexec.Exit())
	return session
}

func getDiskSize(diskPath string) int64 {
	var fsStat syscall.Statfs_t
	Expect(syscall.Statfs(diskPath, &fsStat)).To(Succeed())
	return fsStat.Bsize * int64(fsStat.Blocks)
}
