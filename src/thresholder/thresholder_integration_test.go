package main_test

import (
	"strconv"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"

	"code.cloudfoundry.org/grootfs/commands/config"
	yaml "gopkg.in/yaml.v2"
)

var _ = Describe("Thresholder", func() {
	var (
		gardenGcThreshold   string
		grootGcThreshold    string
		reservedSpace       string
		thresholderCmd      *exec.Cmd
		pathToDisk          string
		pathToGrootfsConfig string
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

	resultingConfig := func() *config.Config {
		gexecStartAndWait(thresholderCmd, GinkgoWriter, GinkgoWriter)
		return configFromFile(pathToGrootfsConfig)
	}

	BeforeEach(func() {
		gardenGcThreshold = "-1"
		grootGcThreshold = "-1"
		reservedSpace = "5000"
		pathToDisk = diskMountPath
		pathToGrootfsConfigAsset := filepath.Join("testassets", "grootfs.yml")
		pathToGrootfsConfig = copyFileToTempFile(pathToGrootfsConfigAsset)
	})

	JustBeforeEach(func() {
		thresholderCmd = exec.Command(thresholderBin, gardenGcThreshold, grootGcThreshold, reservedSpace, pathToDisk, pathToGrootfsConfig)
	})

	AfterEach(func() {
		Expect(os.Remove(pathToGrootfsConfig)).To(Succeed())
	})

	Context("when garden GC threshold is set", func() {
		BeforeEach(func() {
			gardenGcThreshold = "1000"
		})

		It("sets the clean.threshold_bytes value to garden GC threshold", func() {
			Expect(resultingConfig().Clean.ThresholdBytes).To(Equal(bytesToMb(1000)))
		})

		It("sets the create.with_clean value to true", func() {
			Expect(resultingConfig().Create.WithClean).To(BeTrue())
		})
	})

	Context("when groot GC threshold is set", func() {
		BeforeEach(func() {
			grootGcThreshold = "2000"
		})

		Context("and garden GC threshold is NOT set", func() {
			It("sets the clean.threshold_bytes value to groot GC threshold", func() {
				Expect(resultingConfig().Clean.ThresholdBytes).To(Equal(bytesToMb(2000)))
			})

			It("sets the create.with_clean value to true", func() {
				Expect(resultingConfig().Create.WithClean).To(BeTrue())
			})
		})

		Context("and garden GC threshold is set", func() {
			BeforeEach(func() {
				gardenGcThreshold = "3000"
			})

			It("sets the clean.threshold_bytes value to garden GC threshold", func() {
				Expect(resultingConfig().Clean.ThresholdBytes).To(Equal(bytesToMb(3000)))
			})

			It("sets the create.with_clean value to true", func() {
				Expect(resultingConfig().Create.WithClean).To(BeTrue())
			})
		})
	})

	Context("when garden and groot GC thresholds are NOT specified", func() {
		Context("and the reserved space is less than the disk space", func() {
			It("sets the clean.threshold_bytes value to total disk space minus reserved space", func() {
				reservedSpaceInt, err := strconv.ParseInt(reservedSpace, 10, 64)
				Expect(err).NotTo(HaveOccurred())
				Expect(resultingConfig().Clean.ThresholdBytes).To(Equal(diskSize - bytesToMb(reservedSpaceInt)))
			})

			It("sets the create.with_clean value to true", func() {
				Expect(resultingConfig().Create.WithClean).To(BeTrue())
			})
		})

		Context("and the reserved space is greater than the total disk space", func() {
			BeforeEach(func() {
				reservedSpace = strconv.Itoa(1 + int(diskSize/1024/1024))
			})

			It("sets the clean.threshold_bytes value to a negative value", func() {
				Expect(resultingConfig().Clean.ThresholdBytes).To(BeZero())
			})

			It("sets the create.with_clean value to true", func() {
				Expect(resultingConfig().Create.WithClean).To(BeTrue())
			})
		})

		Context("and the reserved space property is -1", func() {
			BeforeEach(func() {
				reservedSpace = "-1"
			})

			It("sets the create.with_clean to false", func() {
				Expect(resultingConfig().Create.WithClean).To(BeFalse())
			})

			It("the clean.threshold_bytes property is not set", func() {
				Expect(resultingConfig().Clean.ThresholdBytes).To(BeZero())
			})
		})
	})

	Context("when the store path cannot be stat", func() {
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

		Context("when garden GC threshold parameter cannot be parsed", func() {
			BeforeEach(func() {
				gardenGcThreshold = "abc"
			})

			exitsNonZeroWithMessage("Garden GC threshold parameter must be a number")
		})

		Context("when groot GC threshold parameter cannot be parsed", func() {
			BeforeEach(func() {
				grootGcThreshold = "abc"
			})

			exitsNonZeroWithMessage("Groot GC threshold parameter must be a number")
		})

		Context("when reserved space parameter cannot be parsed", func() {
			BeforeEach(func() {
				reservedSpace = "abc"
			})

			exitsNonZeroWithMessage("Reserved space parameter must be a number")
		})

		Context("when grootfs configfile does not exist", func() {
			JustBeforeEach(func() {
				thresholderCmd = exec.Command(thresholderBin, "1", "2", "3", "4", "/not/a/path")
			})

			exitsNonZeroWithMessage("Grootfs config parameter must be path to valid grootfs config file")
		})

		Context("when grootfs configfile does not contain valid grootfs config", func() {
			BeforeEach(func() {
				Expect(ioutil.WriteFile(pathToGrootfsConfig, []byte("not yaml"), 0600)).To(Succeed())
			})

			exitsNonZeroWithMessage("Grootfs config parameter must be path to valid grootfs config file")
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

func bytesToMb(bytes int64) int64 {
	return int64(bytes * 1024 * 1024)
}
