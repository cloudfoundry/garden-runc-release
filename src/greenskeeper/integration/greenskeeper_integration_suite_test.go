package greenskeeper_integration_test

import (
	"os/exec"
	"os/user"
	"strconv"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

const (
	vcapID = 5001
	maxID  = 4294967294
)

var (
	gkBin string
)

func TestGreenskeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Greenskeeper Integration Suite")
}

var _ = BeforeSuite(func() {
	var err error
	gkBin, err = gexec.Build("greenskeeper/cmd/greenskeeper", "-mod=vendor")
	Expect(err).ToNot(HaveOccurred())
	ensureVcapUserAndGroup(vcapID)
})

var _ = AfterSuite(func() {
	gexec.CleanupBuildArtifacts()
})

func ensureVcapUserAndGroup(id int) {
	if _, err := user.Lookup("vcap"); err == user.UnknownUserError("vcap") {
		createUser("vcap", id)
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	// ensure the vcap group was implicitely created
	_, err := user.LookupGroup("vcap")
	Expect(err).NotTo(HaveOccurred())
}

func createUser(name string, id int) {
	cmd := exec.Command("useradd", name, "-u", strconv.Itoa(id))
	cmd.Stdout = GinkgoWriter
	cmd.Stderr = GinkgoWriter
	Expect(cmd.Run()).To(Succeed())
}
