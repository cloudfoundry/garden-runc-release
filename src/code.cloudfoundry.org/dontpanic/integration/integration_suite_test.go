package integration_test

import (
	"os"
	"path"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var (
	dontPanicBin string
)

var _ = BeforeSuite(func() {
	bin, err := gexec.Build("code.cloudfoundry.org/dontpanic", "-mod=vendor")
	Expect(err).NotTo(HaveOccurred())
	dontPanicBin = makeBinaryAccessibleToEveryone(bin)
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
	Expect(os.RemoveAll(dontPanicBin)).To(Succeed())
})

func makeBinaryAccessibleToEveryone(binaryPath string) string {
	binaryName := path.Base(binaryPath)

	tempDir, err := os.MkdirTemp("", binaryName)
	Expect(err).NotTo(HaveOccurred())
	Expect(os.Chmod(tempDir, 0755)).To(Succeed())

	newBinaryPath := filepath.Join(tempDir, binaryName)
	Expect(os.Rename(binaryPath, newBinaryPath)).To(Succeed())
	Expect(os.Chmod(newBinaryPath, 0755)).To(Succeed())

	return newBinaryPath
}
