package main_test

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/types"

	"code.cloudfoundry.org/grootfs/commands/config"
	yaml "gopkg.in/yaml.v2"
)

var _ = Describe("Thresholder", func() {
	var (
		reservedSpace       string
		thresholderCmd      *exec.Cmd
		pathToDisk          string
		pathToGrootfsConfig string
		gardenGcThreshold   string
		grootfsGcThreshold  string
	)

	exitsNonZeroWithMessage := func(message string) {
		It("prints an informative error message", func() {
			session := gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			Expect(string(session.Out.Contents())).To(ContainSubstring(message))
		})

		It("exits non zero", func() {
			session := gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			Expect(session.ExitCode()).NotTo(BeZero())
		})
	}

	BeforeEach(func() {
		reservedSpace = "3000"
		pathToDisk = fsMountPoint
		pathToGrootfsConfigAsset := filepath.Join("testassets", "grootfs.yml")
		pathToGrootfsConfig = copyFileToTempFile(pathToGrootfsConfigAsset)
		gardenGcThreshold = "-1"
		grootfsGcThreshold = "-1"
	})

	JustBeforeEach(func() {
		thresholderCmd = exec.Command(thresholderBin, reservedSpace, pathToDisk, pathToGrootfsConfig, gardenGcThreshold, grootfsGcThreshold)
	})

	AfterEach(func() {
		os.Remove(pathToGrootfsConfig)
	})

	Context("when GC threshold is not set (i.e. is a negative value)", func() {
		It("sets clean.threshold_bytes", func() {
			gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			config := configFromFile(pathToGrootfsConfig)

			Expect(config.Clean.ThresholdBytes).To(BeMoreOrLess(diskSize - megabytesToBytes(3000)))
		})

		It("sets init.store_size_bytes", func() {
			gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			config := configFromFile(pathToGrootfsConfig)

			Expect(config.Init.StoreSizeBytes).To(BeMoreOrLess(diskSize - megabytesToBytes(3000)))
		})

		It("sets create.with_clean", func() {
			gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			config := configFromFile(pathToGrootfsConfig)

			Expect(config.Create.WithClean).To(BeTrue())
		})
	})

	Context("when GC threshold is a positive value", func() {
		var config *config.Config

		BeforeEach(func() {
			gardenGcThreshold = "1000"
		})

		JustBeforeEach(func() {
			gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			config = configFromFile(pathToGrootfsConfig)
		})

		It("sets clean.threshold_bytes", func() {
			Expect(config.Clean.ThresholdBytes).To(BeMoreOrLess(megabytesToBytes(1000)))
		})

		It("sets init.store_size_bytes", func() {
			Expect(config.Init.StoreSizeBytes).To(BeMoreOrLess(diskSize))
		})

		It("sets create.with_clean", func() {
			Expect(config.Create.WithClean).To(BeTrue())
		})
	})

	When("the thresholder overrides reserved space to use the whole disk", func() {
		BeforeEach(func() {
			reservedSpace = "1000000000"
		})

		It("uses the whole disk and logs a warning", func() {
			session := gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
			config := configFromFile(pathToGrootfsConfig)

			Expect(config.Init.StoreSizeBytes).To(BeMoreOrLess(diskSize))
			Expect(session).To(gbytes.Say("Warning.*15GB"))
		})
	})

	When("the store path doesn't exist", func() {
		BeforeEach(func() {
			pathToDisk = "/path/to/foo/bar"
			Expect(pathToDisk).NotTo(BeADirectory())
		})

		exitsNonZeroWithMessage(pathToDisk)
	})

	Describe("Parameters validation", func() {
		Context("when not all input args are provided", func() {
			JustBeforeEach(func() {
				thresholderCmd = exec.Command(thresholderBin, "1", "2", "3", "4", "5", "6")
			})

			exitsNonZeroWithMessage("Not all input arguments provided (Expected: 5)")
		})

		Context("when reserved space parameter cannot be parsed", func() {
			BeforeEach(func() {
				reservedSpace = "abc"
			})

			exitsNonZeroWithMessage("Reserved space parameter must be a number")
		})

		Context("when grootfs configfile does not exist", func() {
			BeforeEach(func() {
				pathToGrootfsConfig = "not/a/path"
			})

			exitsNonZeroWithMessage("Grootfs config parameter must be path to valid grootfs config file")
		})

		Context("when grootfs configfile does not contain valid grootfs config", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(pathToGrootfsConfig, []byte("not yaml"), 0o600)).To(Succeed())
			})

			exitsNonZeroWithMessage("Grootfs config parameter must be path to valid grootfs config file")
		})

		Context("when garden gc threshold parameter cannot be parsed", func() {
			BeforeEach(func() {
				gardenGcThreshold = "abc"
			})

			exitsNonZeroWithMessage("Garden GC threshold parameter must be a number")
		})

		Context("when grootfs gc threshold parameter cannot be parsed", func() {
			BeforeEach(func() {
				grootfsGcThreshold = "abc"
			})

			exitsNonZeroWithMessage("GrootFS GC threshold parameter must be a number")
		})
	})
})

func copyFileToTempFile(src string) string {
	fileContents, err := ioutil.ReadFile(src)
	Expect(err).NotTo(HaveOccurred())

	tempFile, err := ioutil.TempFile("", "")
	Expect(err).NotTo(HaveOccurred())
	defer tempFile.Close()

	_, err = io.Copy(tempFile, bytes.NewReader(fileContents))
	Expect(err).NotTo(HaveOccurred())

	return tempFile.Name()
}

func configFromFile(path string) *config.Config {
	conf, err := ioutil.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())

	var c config.Config
	Expect(yaml.Unmarshal(conf, &c)).To(Succeed())

	return &c
}

func megabytesToBytes(megabytes int64) int64 {
	return int64(megabytes * 1024 * 1024)
}

func BeMoreOrLess(n int64) types.GomegaMatcher {
	return BeNumerically("~", n, float64(n)*0.001)
}
