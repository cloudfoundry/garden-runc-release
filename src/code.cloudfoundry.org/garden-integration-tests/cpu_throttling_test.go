package garden_integration_tests_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	sigar "github.com/cloudfoundry/gosigar"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CPU Throttling", func() {
	var (
		containerPort uint32

		badContainer     garden.Container
		badContainerPort uint32
	)

	BeforeEach(func() {
		skipIfCpuThrottlingNotEnabled()

		imageRef = garden.ImageRef{URI: "docker:///cloudfoundry/garden-rootfs"}
		//We set the weight to the system memory in order to make sure that the container would be never punished
		limits = garden.Limits{CPU: garden.CPULimits{Weight: totalMemoryInMegabytes()}}
	})

	JustBeforeEach(func() {
		var err error
		containerPort, _, err = container.NetIn(0, 8080)
		Expect(err).NotTo(HaveOccurred())
		startSpinnerApp(container, containerPort)

		badContainer, err = gardenClient.Create(garden.ContainerSpec{
			Image: imageRef,
			//We set the weight to a pretty low number in order to make sure that when spinning, the container would
			// be punished to the bad cgroup and would never get out form there
			Limits: garden.Limits{CPU: garden.CPULimits{Weight: 100}},
		})
		Expect(err).NotTo(HaveOccurred())

		badContainerPort, _, err = badContainer.NetIn(0, 8080)
		Expect(err).NotTo(HaveOccurred())
		startSpinnerApp(badContainer, badContainerPort)
	})

	AfterEach(func() {
		Expect(destroyContainer(badContainer)).To(Succeed())
	})

	Context("CPU-intensive application is punished to the bad cgroup (because it is way over its entitlement)", func() {
		JustBeforeEach(func() {
			spinToPunish(badContainer, badContainerPort)
		})

		Context("and another application wants to spike", func() {
			var beforeUsage uint64
			JustBeforeEach(func() {
				initialUsage, _, err := getCPUUsageAndEntitlement(container)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(5 * time.Second)
				finalUsage, _, err := getCPUUsageAndEntitlement(container)
				Expect(err).ToNot(HaveOccurred())
				beforeUsage = finalUsage - initialUsage
				Expect(spin(container, containerPort)).To(Succeed())
			})

			It("allows the previously idle application to spike", func() {
				initialUsage, _, err := getCPUUsageAndEntitlement(container)
				Expect(err).ToNot(HaveOccurred())
				time.Sleep(5 * time.Second)
				finalUsage, _, err := getCPUUsageAndEntitlement(container)
				Expect(err).ToNot(HaveOccurred())
				afterUsage := finalUsage - initialUsage
				Expect(afterUsage).To(BeNumerically(">", beforeUsage*10))
			})
		})
	})
})

func skipIfCpuThrottlingNotEnabled() {
	if os.Getenv("CPU_THROTTLING_ENABLED") == "true" {
		return
	}

	Skip("CPU throttling is not enabled")
}

func externalIP(container garden.Container) string {
	properties, err := container.Properties()
	Expect(err).NotTo(HaveOccurred())
	return properties[gardener.ExternalIPKey]
}

func spin(container garden.Container, containerPort uint32) error {
	if _, err := httpGet(fmt.Sprintf("http://%s:%d/spin", externalIP(container), containerPort)); err != nil {
		return fmt.Errorf("spin %s failed: %+v", container.Handle(), err)
	}

	return nil
}

func spinToPunish(container garden.Container, port uint32) {
	ExpectWithOffset(1, spin(container, port)).To(Succeed())
	EventuallyWithOffset(1, punished(container, port), "60s").Should(BeTrue())
}

func startSpinnerApp(container garden.Container, containerPort uint32) {
	_, err := container.Run(garden.ProcessSpec{Path: "/bin/throttled-or-not"}, garden.ProcessIO{})
	Expect(err).NotTo(HaveOccurred())

	Eventually(func() (string, error) {
		return httpGet(fmt.Sprintf("http://%s:%d/ping", externalIP(container), containerPort))
	}).Should(Equal("pong"))
	ensureInitialSpikeIsOver(container, containerPort)
}

func ensureInitialSpikeIsOver(container garden.Container, port uint32) {
	// Wait for the usage to drop below 0.01 (i.e. the container is done initialisizing and is idle)
	// and eventually get into the good cgroup
	Eventually(currentUsage(container), "1m", "1s").Should(BeNumerically("<", 0.01))
	Eventually(punished(container, port), "1m", "1s").Should(BeFalse())
}

func getCPUUsageAndEntitlement(container garden.Container) (uint64, uint64, error) {
	metrics, err := container.Metrics()
	if err != nil {
		return 0, 0, err
	}

	return metrics.CPUStat.Usage, metrics.CPUEntitlement, nil
}

func currentUsage(container garden.Container) func() (float64, error) {
	return func() (float64, error) {
		firstUsage, firstEntitlement, err := getCPUUsageAndEntitlement(container)
		if err != nil {
			return 0, nil
		}

		time.Sleep(1 * time.Second)

		secondUsage, secondEntitlement, err := getCPUUsageAndEntitlement(container)
		if err != nil {
			return 0, nil
		}

		deltaUsage := secondUsage - firstUsage
		deltaEntitlement := secondEntitlement - firstEntitlement

		result := float64(deltaUsage) / float64(deltaEntitlement)
		// writeToGinkgo(fmt.Sprintf("[%s] usage: %f\n", container.Handle(), result))
		return result, nil
	}
}

func isPunished(container garden.Container, containerPort uint32) (bool, error) {
	cgroup, err := httpGet(fmt.Sprintf("http://%s:%d/cpucgroup", externalIP(container), containerPort))
	if err != nil {
		return false, err
	}

	return strings.HasSuffix(cgroup, filepath.Join(cgroups.BadCgroupName, container.Handle())), nil
}

func punished(container garden.Container, containerPort uint32) func() (bool, error) {
	return func() (bool, error) {
		return isPunished(container, containerPort)
	}
}

func totalMemoryInMegabytes() uint64 {
	mem := sigar.Mem{}
	ExpectWithOffset(1, mem.Get()).To(Succeed())
	return mem.Total / 1024 / 1024
}
