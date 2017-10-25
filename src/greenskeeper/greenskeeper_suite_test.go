package greenskeeper

import (
	"os/exec"
	"os/user"
	"testing"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gexec"
)

func TestGreenskeeper(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Greenskeeper Suite")
}

var _ = BeforeSuite(func() {
	if _, err := user.LookupGroup("mewtwo"); err == user.UnknownGroupError("mewtwo") {
		createGroup("mewtwo")
	} else {
		Expect(err).NotTo(HaveOccurred())
	}

	if _, err := user.Lookup("mew"); err == user.UnknownUserError("mew") {
		createUser("mew", "mewtwo")
	} else {
		Expect(err).NotTo(HaveOccurred())
	}
})

func createGroup(name string) {
	cmd := exec.Command("groupadd", name)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, time.Second*5).Should(gexec.Exit(0))
}

func createUser(name, group string) {
	cmd := exec.Command("useradd", name, "-G", group)
	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, time.Second*5).Should(gexec.Exit(0))
}
