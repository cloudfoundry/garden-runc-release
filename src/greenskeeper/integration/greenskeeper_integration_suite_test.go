package greenskeeper_integration_test

import (
	"io/ioutil"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

var (
	gkBin  string
	tmpDir string
)

func TestGreenskeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Greenskeeper Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	gkBin, err = gexec.Build("greenskeeper/cmd/greenskeeper")
	Expect(err).ToNot(HaveOccurred())

	tmpDir, err = ioutil.TempDir("", "")
	Expect(err).ToNot(HaveOccurred())
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
	os.RemoveAll(tmpDir)
	Expect(tmpDir).NotTo(BeADirectory())
})
