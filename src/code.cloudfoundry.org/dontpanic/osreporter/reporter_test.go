package osreporter_test

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/dontpanic/osreporter"
	"code.cloudfoundry.org/dontpanic/osreporter/osreporterfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Reporter", func() {
	var (
		runner       osreporter.Reporter
		outputWriter io.Writer
		collectorOne *osreporterfakes.FakeCollector
		collectorTwo *osreporterfakes.FakeCollector
	)

	BeforeEach(func() {
		outputWriter = GinkgoWriter
	})

	var (
		reportDir string
	)

	BeforeEach(func() {
		var err error

		reportDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(reportDir, "hello"), []byte("hello"), 0644)).To(Succeed())

		outputWriter = gbytes.NewBuffer()

		collectorOne = new(osreporterfakes.FakeCollector)
		collectorTwo = new(osreporterfakes.FakeCollector)

		runner = osreporter.New(reportDir, outputWriter)
		runner.RegisterCollector("collector-one", collectorOne)
		runner.RegisterNoisyCollector("collector-two", collectorTwo)
	})

	AfterEach(func() {
		os.RemoveAll(reportDir)
	})

	It("zips the report dir", func() {
		Expect(runner.Run()).To(Succeed())
		Expect(reportDir + ".tar.gz").To(BeAnExistingFile())
		Expect(tarballFileContents(reportDir+".tar.gz", "hello")).To(Equal([]byte("hello")))
		Expect(fileType(reportDir + ".tar.gz")).To(ContainSubstring("gzip"))
	})

	It("runs all collectors in sequence", func() {
		Expect(runner.Run()).To(Succeed())

		Expect(outputWriter).To(gbytes.Say("## collector-one"))
		Expect(outputWriter).To(gbytes.Say("## collector-two"))

		Expect(collectorOne.RunCallCount()).To(Equal(1))
		Expect(collectorTwo.RunCallCount()).To(Equal(1))

		_, actualDstPath, actualStdout := collectorOne.RunArgsForCall(0)
		Expect(actualDstPath).To(Equal(reportDir))
		Expect(actualStdout).To(Equal(io.Discard))
	})

	When("registering a collector with stdout printing", func() {
		It("provides stdout as the output writer", func() {
			Expect(runner.Run()).To(Succeed())

			_, _, actualStdout := collectorTwo.RunArgsForCall(0)
			Expect(actualStdout).To(Equal(outputWriter))
		})
	})

	When("a collector returns an error", func() {
		BeforeEach(func() {
			collectorOne.RunReturns(errors.New("collector-one-error"))
		})

		It("notifies failure", func() {
			Expect(runner.Run()).To(Succeed())
			Expect(outputWriter).To(gbytes.Say(">> collector-one failed: collector-one-error"))
		})
	})

	When("a collector takes too long to run", func() {
		BeforeEach(func() {
			collectorOne.RunReturns(context.DeadlineExceeded)
		})

		It("times out with the function timeout helper", func() {
			Expect(runner.Run()).To(Succeed())
			Expect(outputWriter).To(gbytes.Say("timed out after 10s"))
		})
	})
})

func tarballFileContents(tarballPath, filePath string) []byte {
	extractedOsReportPath := strings.TrimSuffix(filepath.Base(tarballPath), ".tar.gz")
	osDir := filepath.Base(extractedOsReportPath)

	cmd := exec.Command("tar", "xzf", tarballPath, filepath.Join(osDir, filePath), "-O")
	out, err := cmd.Output()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	return out
}

func fileType(path string) string {
	cmd := exec.Command("file", path)
	out, err := cmd.Output()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	return string(out)
}
