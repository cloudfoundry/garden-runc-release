package greenskeeper_integration_test

import (
	"os/exec"
	"os/user"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"

	"testing"
)

const (
	vcapID = 5001
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
	gkBin, err = gexec.Build("greenskeeper/cmd/greenskeeper")
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
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, time.Second*5).Should(gexec.Exit(0))
}
