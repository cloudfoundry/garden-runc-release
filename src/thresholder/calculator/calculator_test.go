package calculator_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"thresholder/calculator"
)

var _ = Describe("calculator", func() {
	var (
		reservedSpace int64
		diskSize      int64
		minStoreSize  int64
	)

	BeforeEach(func() {
		diskSize = 10
		minStoreSize = 3
	})

	Describe("CalculateStoreSize", func() {
		var storeSize int64

		BeforeEach(func() {
			reservedSpace = 4
		})

		JustBeforeEach(func() {
			storeSize = calculator.CalculateStoreSize(reservedSpace, diskSize, minStoreSize)
		})

		It("returns disk size - reserved size", func() {
			Expect(storeSize).To(Equal(int64(6)))
		})

		When("disk size - reserved size <= min store size", func() {
			BeforeEach(func() {
				reservedSpace = 8
			})

			It("returns the disk size", func() {
				Expect(storeSize).To(Equal(int64(10)))
			})
		})
	})

	Describe("CalculateGCThreshold", func() {
		var threshold int64

		BeforeEach(func() {
			reservedSpace = 4
		})

		JustBeforeEach(func() {
			threshold = calculator.CalculateGCThreshold(reservedSpace, diskSize, minStoreSize)
		})

		It("returns (disk size - reserved size)", func() {
			Expect(threshold).To(Equal(int64(6)))
		})

		When("reserved size > disk size", func() {
			BeforeEach(func() {
				reservedSpace = 12
			})

			It("returns 0", func() {
				Expect(threshold).To(BeZero())
			})
		})
	})

	Describe("ShouldCollectGarbageOnCreate", func() {
		var cleanOnStart bool

		JustBeforeEach(func() {
			cleanOnStart = calculator.ShouldCollectGarbageOnCreate(reservedSpace)
		})

		When("reserved size >= 0", func() {
			BeforeEach(func() {
				reservedSpace = 1
			})

			It("returns true", func() {
				Expect(cleanOnStart).To(Equal(true))
			})
		})

		When("reserved size < 0", func() {
			BeforeEach(func() {
				reservedSpace = -1
			})

			It("returns false", func() {
				Expect(cleanOnStart).To(Equal(false))
			})
		})
	})
})
