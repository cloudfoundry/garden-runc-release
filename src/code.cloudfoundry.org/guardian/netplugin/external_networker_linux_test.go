package netplugin_test

import (
	"net"
	"os"
	"os/exec"
	"syscall"

	"code.cloudfoundry.org/commandrunner/fake_command_runner"
	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/kawasaki/kawasakifakes"
	"code.cloudfoundry.org/guardian/netplugin"
	"code.cloudfoundry.org/guardian/properties"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/vishvananda/netlink"
	"github.com/vishvananda/netns"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExternalNetworker loopback", func() {
	var (
		sleeper *exec.Cmd
		handle  *netlink.Handle
	)

	BeforeEach(func() {
		if os.Geteuid() != 0 {
			Skip("creating a network namespace requires root")
		}

		sleeper = exec.Command("sleep", "60")
		sleeper.SysProcAttr = &syscall.SysProcAttr{Cloneflags: syscall.CLONE_NEWNET}
		Expect(sleeper.Start()).To(Succeed())
		DeferCleanup(func() {
			Expect(sleeper.Process.Kill()).To(Succeed())
			_ = sleeper.Wait()
		})

		containerNS, err := netns.GetFromPid(sleeper.Process.Pid)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(containerNS.Close)
		handle, err = netlink.NewHandleAt(containerNS)
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(handle.Close)
	})

	loopbackIsUp := func() bool {
		lo, err := handle.LinkByName("lo")
		Expect(err).NotTo(HaveOccurred())
		return lo.Attrs().Flags&net.FlagUp != 0
	}

	It("brings up the loopback interface in the container's network namespace", func() {
		Expect(loopbackIsUp()).To(BeFalse())

		plugin := netplugin.New(
			fake_command_runner.New(),
			properties.NewManager(),
			net.ParseIP("1.2.3.4"),
			nil,
			nil,
			new(kawasakifakes.FakeDnsResolvConfigurer),
			"some/path",
			nil,
			new(kawasakifakes.FakeNetworkDepot),
		)
		spec := garden.ContainerSpec{Handle: "some-handle", Network: "potato"}
		Expect(plugin.Network(lagertest.NewTestLogger("test"), spec, sleeper.Process.Pid)).To(Succeed())

		Expect(loopbackIsUp()).To(BeTrue())
	})
})
