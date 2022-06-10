package calculator_test

import (
	"thresholder/calculator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	diskSize int64
	calc     calculator.Calculator
)

var _ = Describe("modern calculator", func() {
	var (
		reservedSpace int64
		minStoreSize  int64
	)

	BeforeEach(func() {
		diskSize = 10
		minStoreSize = 3
	})

	JustBeforeEach(func() {
		calc = calculator.NewModernCalculator(reservedSpace, diskSize, minStoreSize)
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
	})

	Describe("CalculateGCThreshold", func() {
		var threshold int64

		BeforeEach(func() {
			reservedSpace = 4
		})

		JustBeforeEach(func() {
			threshold = calc.CalculateGCThreshold()
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
