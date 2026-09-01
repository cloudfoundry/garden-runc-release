package calculator_test

import (
	"code.cloudfoundry.org/thresholder/calculator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	diskSize  int64
	routineGC bool
	calc      calculator.Calculator
)

var _ = Describe("modern calculator", func() {
	var (
		reservedSpace     int64
		minStoreSize      int64
		gcThresholdFactor float64
	)

	BeforeEach(func() {
		diskSize = 10
		routineGC = false
		minStoreSize = 3
		gcThresholdFactor = 1.0
	})

	JustBeforeEach(func() {
		calc = calculator.NewModernCalculator(reservedSpace, diskSize, minStoreSize, routineGC, gcThresholdFactor)
	})

	Describe("CalculateStoreSize", func() {
		var storeSize int64

		BeforeEach(func() {
			reservedSpace = 4
		})

		JustBeforeEach(func() {
			storeSize = calc.CalculateStoreSize()
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

		When("routineGC is true", func() {
			BeforeEach(func() {
				routineGC = true
			})

			It("acts as though the reservedSpace is equal to diskSize and returns the diskSize", func() {
				Expect(storeSize).To(Equal(diskSize))
			})
		})

		When("the GC threshold factor is less than 1", func() {
			BeforeEach(func() {
				gcThresholdFactor = 0.5
			})

			It("does not affect the store size", func() {
				Expect(storeSize).To(Equal(int64(6)))
			})
		})
	})

	Describe("CalculateGCThreshold", func() {
		var threshold int64

		BeforeEach(func() {
			reservedSpace = 4
		})

		JustBeforeEach(func() {
			threshold = calc.CalculateGCThreshold()
		})

		It("returns (disk size - reserved size) when factor is 1", func() {
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

		When("routineGC is true", func() {
			BeforeEach(func() {
				routineGC = true
			})

			It("acts as though the reservedSpace is equal to diskSize and returns 0", func() {
				Expect(threshold).To(BeZero())
			})
		})

		When("reserved size is -1", func() {
			BeforeEach(func() {
				reservedSpace = -1
			})

			It("does not subtract the sentinel value from the disk size", func() {
				Expect(threshold).To(Equal(diskSize))
			})

			When("the factor is less than 1", func() {
				BeforeEach(func() {
					gcThresholdFactor = 0.5
				})

				It("scales the full disk size by the factor", func() {
					Expect(threshold).To(Equal(int64(5)))
				})
			})
		})

		When("the GC threshold factor is 0.5", func() {
			BeforeEach(func() {
				gcThresholdFactor = 0.5
			})

			It("returns half of (disk size - reserved size)", func() {
				Expect(threshold).To(Equal(int64(3)))
			})
		})

		When("the GC threshold factor is 1", func() {
			BeforeEach(func() {
				gcThresholdFactor = 1.0
			})

			It("preserves the historical behaviour", func() {
				Expect(threshold).To(Equal(int64(6)))
			})
		})

		When("the GC threshold factor produces a fractional result", func() {
			BeforeEach(func() {
				diskSize = 10
				reservedSpace = 0
				gcThresholdFactor = 0.33
			})

			It("floors the threshold to a whole number of bytes", func() {
				Expect(threshold).To(Equal(int64(3)))
			})
		})

		When("the GC threshold factor combines with routineGC", func() {
			BeforeEach(func() {
				routineGC = true
				gcThresholdFactor = 0.5
			})

			It("still returns 0 because reservedSpace is forced to diskSize", func() {
				Expect(threshold).To(BeZero())
			})
		})

		When("the GC threshold factor is invalid (<= 0)", func() {
			BeforeEach(func() {
				gcThresholdFactor = 0
			})

			It("falls back to a factor of 1.0 to preserve historical behaviour", func() {
				Expect(threshold).To(Equal(int64(6)))
			})
		})

		When("the GC threshold factor is invalid (> 1)", func() {
			BeforeEach(func() {
				gcThresholdFactor = 2.5
			})

			It("falls back to a factor of 1.0 to preserve historical behaviour", func() {
				Expect(threshold).To(Equal(int64(6)))
			})
		})

		When("the GC threshold factor is negative", func() {
			BeforeEach(func() {
				gcThresholdFactor = -0.25
			})

			It("falls back to a factor of 1.0 to preserve historical behaviour", func() {
				Expect(threshold).To(Equal(int64(6)))
			})
		})
	})

	Describe("ShouldCollectGarbageOnCreate", func() {
		var cleanOnStart bool

		JustBeforeEach(func() {
			cleanOnStart = calc.ShouldCollectGarbageOnCreate()
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

		When("routineGC is true", func() {
			BeforeEach(func() {
				routineGC = true
			})

			It("acts as though the reservedSpace is equal to diskSize and returns true", func() {
				Expect(cleanOnStart).To(BeTrue())
			})
		})

		When("the GC threshold factor is less than 1", func() {
			BeforeEach(func() {
				reservedSpace = 1
				gcThresholdFactor = 0.5
			})

			It("does not affect the result", func() {
				Expect(cleanOnStart).To(BeTrue())
			})
		})
	})
})

var _ = Describe("old fashioned calculator", func() {
	var (
		grootfsGcThreshold int64
		gardenGcThreshold  int64
	)

	BeforeEach(func() {
		grootfsGcThreshold = -1
		gardenGcThreshold = -1
		diskSize = 10
	})

	JustBeforeEach(func() {
		calc = calculator.NewOldFashionedCalculator(diskSize, gardenGcThreshold, grootfsGcThreshold)
	})

	Describe("CalculateGCThreshold", func() {
		When("grootfs GC threshold is greater than zero", func() {
			BeforeEach(func() {
				grootfsGcThreshold = 4
			})

			It("returns the grootfs GC threshold", func() {
				Expect(calc.CalculateGCThreshold()).To(Equal(int64(4)))
			})
		})

		When("garden GC threshold is greater than zero", func() {
			BeforeEach(func() {
				gardenGcThreshold = 5
			})

			It("returns the garden GC threshold", func() {
				Expect(calc.CalculateGCThreshold()).To(Equal(int64(5)))
			})
		})

		When("both garden and grootfs GC thresholds are greater than zero", func() {
			BeforeEach(func() {
				grootfsGcThreshold = 4
				gardenGcThreshold = 5
			})

			It("returns the garden GC threshold", func() {
				Expect(calc.CalculateGCThreshold()).To(Equal(int64(5)))
			})
		})

		When("gc threshold is a negative value", func() {
			It("returns threshold greater than the disk size", func() {
				Expect(calc.CalculateGCThreshold()).To(BeNumerically(">", diskSize))
			})
		})
	})

	Describe("ShouldCollectGarbageOnCreate", func() {
		When("gc threshold is a negative value", func() {
			It("should not garbage collect on create", func() {
				Expect(calc.ShouldCollectGarbageOnCreate()).To(BeFalse())
			})
		})

		When("gc threshold is >= 0", func() {
			BeforeEach(func() {
				gardenGcThreshold = 0
			})

			It("should garbage collect on create", func() {
				Expect(calc.ShouldCollectGarbageOnCreate()).To(BeTrue())
			})
		})
	})

	Describe("CalculateStoreSize", func() {
		It("equals the disk size", func() {
			Expect(calc.CalculateStoreSize()).To(Equal(diskSize))
		})
	})
})
