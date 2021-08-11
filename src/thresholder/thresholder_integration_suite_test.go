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

func TestThresholder(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Thresholder Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	thresholderBin, err = gexec.Build("thresholder", "-mod=vendor")
	Expect(err).ToNot(HaveOccurred())

	diskSize = getDiskAvailableSpace("/")
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
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
