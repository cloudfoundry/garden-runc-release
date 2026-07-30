package garden_integration_tests_test

import (
	"code.cloudfoundry.org/garden"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Capacity", func() {
	JustBeforeEach(func() {
		skipIfWoot("Groot does not support capacity yet")
	})

	It("returns the memory in bytes", func() {
		Eventually(func() uint64 {
			return capacity().MemoryInBytes
		}).Should(BeNumerically(">", 0))
	})

	It("returns the disk size in bytes", func() {
		Eventually(func() uint64 {
			return capacity().DiskInBytes
		}).Should(BeNumerically(">", 0))
	})

	It("returns the schedulable disk capacity in bytes", func() {
		Eventually(func() uint64 {
			return capacity().SchedulableDiskInBytes
		}).Should(BeNumerically(">", 0))
	})

	It("returns the maximum number of containers", func() {
		Eventually(func() uint64 {
			return capacity().MaxContainers
		}).Should(BeNumerically(">", 0))
	})
})

func capacity() garden.Capacity {
	capacity, err := gardenClient.Capacity()
	Expect(err).NotTo(HaveOccurred())
	return capacity
}
