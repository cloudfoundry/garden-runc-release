package grootfs_test

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/dontpanic/collectors/grootfs"
	"code.cloudfoundry.org/dontpanic/collectors/grootfs/grootfsfakes"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

func contains(list []string, item string) bool {
	for _, s := range list {
		if strings.Contains(s, item) {
			return true
		}
	}
	return false
}

var _ = Describe("Grootfs", func() {

	var (
		collector      grootfs.UsageCollector
		tmpDir         string
		configFilePath string
		fakeRunner     *grootfsfakes.FakeCommandRunner
		ctx            context.Context
		stdout         io.Writer
		runError       error
		usageFilePath  string

		configFileTemplate = `
store: %s/unprivileged
tardis_bin: /var/vcap/packages/grootfs/bin/tardis
`
	)

	BeforeEach(func() {
		var err error
		tmpDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())

		configFilePath = filepath.Join(tmpDir, "config.yml")
		err = os.WriteFile(configFilePath, []byte(fmt.Sprintf(configFileTemplate, tmpDir)), 0644)
		Expect(err).NotTo(HaveOccurred())

		fakeRunner = new(grootfsfakes.FakeCommandRunner)
		collector = grootfs.NewUsageCollector(configFilePath, fakeRunner)
		ctx = context.TODO()
		stdout = gbytes.NewBuffer()
		usageFilePath = filepath.Join(tmpDir, "grootfs", "unprivileged-usage.txt")

		fakeRunner.RunStub = func(ctx context.Context, cmd string, args ...string) ([]byte, error) {
			switch cmd {
			case "du":
				if contains(args, ".backing-store") {
					if contains(args, "--apparent-size") {
						return []byte("123456789\tfoo/bar/sha\n"), nil
					}
					return []byte("12345678\tfoo/bar/sha\n"), nil
				}
				return []byte("123456\tfoo/bar/sha\n"), nil
			case "/var/vcap/packages/grootfs/bin/grootfs":
				return []byte(`{"disk_usage": {"exclusive_bytes_used": 3040, "quota_size_bytes": 6000}}`), nil
			default:
				Fail("unexpected " + args[0])
			}
			return nil, nil
		}
	})

	AfterEach(func() {
		os.RemoveAll(tmpDir)
	})

	JustBeforeEach(func() {
		runError = collector.Run(ctx, tmpDir, stdout)
		Expect(runError).NotTo(HaveOccurred())
	})

	Context("usage output", func() {
		BeforeEach(func() {

			linkDir := filepath.Join(tmpDir, "unprivileged", "l")
			metaDir := filepath.Join(tmpDir, "unprivileged", "meta")
			dependenciesDir := filepath.Join(tmpDir, "unprivileged", "meta", "dependencies")
			imageDir1 := filepath.Join(tmpDir, "unprivileged", "images", "image1")
			imageDir2 := filepath.Join(tmpDir, "unprivileged", "images", "image2")
			volumeDir1 := filepath.Join(tmpDir, "unprivileged", "volumes", "vol123")
			volumeDir2 := filepath.Join(tmpDir, "unprivileged", "volumes", "vol456")
			volumeDir3 := filepath.Join(tmpDir, "unprivileged", "volumes", "vol789")

			Expect(os.MkdirAll(linkDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(metaDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(dependenciesDir, 0755)).To(Succeed())
			Expect(os.MkdirAll(imageDir1, 0755)).To(Succeed())
			Expect(os.MkdirAll(imageDir2, 0755)).To(Succeed())
			Expect(os.MkdirAll(volumeDir1, 0755)).To(Succeed())
			Expect(os.MkdirAll(volumeDir2, 0755)).To(Succeed())
			Expect(os.MkdirAll(volumeDir3, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dependenciesDir, "image:image1.json"), []byte(`["vol123"]`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(dependenciesDir, "image:image2.json"), []byte(`["vol456"]`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(metaDir, "volume-vol123"), []byte(`{"Size": 1024}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(metaDir, "volume-vol456"), []byte(`{"Size": 2048}`), 0644)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(metaDir, "volume-vol789"), []byte(`{"Size": 4096}`), 0644)).To(Succeed())
		})

		It("includes stats from calling du on volumes", func() {
			Expect(contents(usageFilePath)).To(ContainSubstring("volumes-total-on-disk:               123456 bytes"))
			Expect(contents(usageFilePath)).To(ContainSubstring("volumes-used-on-disk:                246912 bytes"))
			Expect(contents(usageFilePath)).To(ContainSubstring("volumes-unused-on-disk:              123456 bytes"))
		})

		It("includes stats from reading meta on volumes", func() {
			Expect(contents(usageFilePath)).To(ContainSubstring("volumes-used-reported:                 3072 bytes"))
			Expect(contents(usageFilePath)).To(ContainSubstring("volumes-unused-reported:               4096 bytes"))
		})

		It("includes image volumes size in usage", func() {
			Expect(contents(usageFilePath)).To(ContainSubstring("images-exclusive:                      6080 bytes"))
		})

		It("includes current size of backing file in usage", func() {
			Expect(contents(usageFilePath)).To(ContainSubstring("backing-store-actual-size:         12345678 bytes"))
			Expect(contents(usageFilePath)).To(ContainSubstring("backing-store-max-size:           123456789 bytes"))
		})

		It("includes quotas size", func() {
			Expect(contents(usageFilePath)).To(ContainSubstring("quotas-size:                          12000 bytes"))
		})

	})
})

func contents(path string) string {
	b, err := os.ReadFile(path)
	Expect(err).NotTo(HaveOccurred())
	return string(b)
}
