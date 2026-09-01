package measurements_test

import (
	. "code.cloudfoundry.org/garden-performance-acceptance-tests/measurements"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Measurements", func() {
	var measurements Measurements

	Describe("Average", func() {
		Context("when there are no measurements", func() {
			BeforeEach(func() {
				measurements = []float64{}
			})

			It("should return an error", func() {
				_, err := measurements.Average()
				Expect(err).To(MatchError("no measurements recorded, cannot compute average"))
			})
		})

		Context("when there is one measurement", func() {
			BeforeEach(func() {
				measurements = []float64{1.0}
			})

			It("should return that measurement", func() {
				avg, err := measurements.Average()
				Expect(err).NotTo(HaveOccurred())

				Expect(avg).To(Equal(1.0))
			})
		})

		Context("when there are many measurements", func() {
			BeforeEach(func() {
				measurements = []float64{1.0, 2.0, 6.0}
			})

			It("should return the average of the two measurements", func() {
				avg, err := measurements.Average()
				Expect(err).NotTo(HaveOccurred())

				Expect(avg).To(Equal(3.0))
			})
		})
	})
})
