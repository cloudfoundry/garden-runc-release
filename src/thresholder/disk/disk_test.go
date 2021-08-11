package disk_test

import (
	"errors"
	"thresholder/disk"
	"thresholder/disk/diskfakes"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Meter", func() {
	var (
		fs             *diskfakes.FakeFS
		meter          disk.Meter
		availableSpace int64
		err            error
	)

	BeforeEach(func() {
		fs = new(diskfakes.FakeFS)
		fs.StatReturns(disk.Stat{
			AvailableBlocks: 6,
			BlockSize:       7,
		}, nil)

		meter = disk.NewMeterWithFS(fs)
	})

	JustBeforeEach(func() {
		availableSpace, err = meter.GetAvailableSpace("/some/path")
	})

	It("returns the available space on the FS containing the provided path", func() {
		Expect(err).NotTo(HaveOccurred())
		Expect(availableSpace).To(Equal(int64(42)))
		Expect(fs.StatCallCount()).To(Equal(1))
		Expect(fs.StatArgsForCall(0)).To(Equal("/some/path"))
	})

	When("statting fails", func() {
		BeforeEach(func() {
			fs.StatReturns(disk.Stat{}, errors.New("stat-error"))
		})

		It("fails", func() {
			Expect(err).To(MatchError("cannot stat /some/path: stat-error"))
		})
	})
})
