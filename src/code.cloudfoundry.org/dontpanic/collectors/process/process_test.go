package process_test

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"context"

	"code.cloudfoundry.org/dontpanic/collectors/process"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Process", func() {
	var (
		ctx              context.Context
		stdout           io.Writer
		destDir          string
		processCollector process.Collector
		runErr           error
	)

	BeforeEach(func() {
		ctx = context.TODO()
		stdout = gbytes.NewBuffer()

		var err error
		destDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())
		processCollector = process.NewCollector("procDataDir")
	})

	JustBeforeEach(func() {
		runErr = processCollector.Run(ctx, destDir, stdout)
	})

	It("collects info for every running process", func() {
		Expect(runErr).NotTo(HaveOccurred())

		Expect(filepath.Join(destDir, "procDataDir", "1", "fd")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", "1", "ns")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", "1", "cgroup")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", "1", "status")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", "1", "fd")).To(BeAnExistingFile())

		thisPid := os.Getpid()
		Expect(filepath.Join(destDir, "procDataDir", strconv.Itoa(thisPid), "fd")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", strconv.Itoa(thisPid), "ns")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", strconv.Itoa(thisPid), "cgroup")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", strconv.Itoa(thisPid), "status")).To(BeAnExistingFile())
		Expect(filepath.Join(destDir, "procDataDir", strconv.Itoa(thisPid), "stack")).To(BeAnExistingFile())
	})

	Context("when run fails", func() {
		var cancel context.CancelFunc

		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
			cancel()
		})

		It("propagates the error", func() {
			Expect(runErr).To(Equal(context.DeadlineExceeded))
		})
	})
})
