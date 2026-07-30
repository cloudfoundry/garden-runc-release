package main_test

import (
	"bytes"
	"io"
	"os/exec"
	"syscall"
	"testing"

	"github.com/BurntSushi/toml"
	. "github.com/onsi/ginkgo/v2"
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

type data struct {
	Binary string
	Size   int64
}

var _ = SynchronizedBeforeSuite(func() []byte {
	binary, err := gexec.Build("thresholder", "-mod=vendor")
	Expect(err).ToNot(HaveOccurred())

	createAndMountFilesystem(fsFile, fsSize, fsMountPoint)
	size := getDiskAvailableSpace(fsMountPoint)
	d := data{
		Binary: binary,
		Size:   size,
	}
	return jsonMarshal(d)

}, func(input []byte) {
	d := new(data)
	jsonUnmarshal(input, d)
	thresholderBin = d.Binary
	diskSize = d.Size
})

var _ = SynchronizedAfterSuite(func() {}, func() {
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
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running truncate failed")

	err = exec.Command("mkfs.ext4", filename).Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running mkfs.ext4 failed")

	err = exec.Command("mkdir", "-p", mntPoint).Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running mkdir failed")

	err = exec.Command("mount", filename, mntPoint).Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running mount failed")
}

func cleanupFilesystem(filename, mntPoint string) {
	err := exec.Command("umount", mntPoint).Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running umount failed")

	err = exec.Command("rm", "-rf", filename, mntPoint).Run()
	ExpectWithOffset(1, err).NotTo(HaveOccurred(), "running rm -rf failed")
}

func jsonMarshal(v interface{}) []byte {
	buf := bytes.NewBuffer([]byte{})
	Expect(toml.NewEncoder(buf).Encode(v)).To(Succeed())
	return buf.Bytes()
}

func jsonUnmarshal(data []byte, v interface{}) {
	Expect(toml.Unmarshal(data, v)).To(Succeed())
}
