package file_test

import (
	"context"
	"io"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"

	"code.cloudfoundry.org/dontpanic/collectors/file"
)

var _ = Describe("file.Collector", func() {
	var (
		ctx            context.Context
		destinationDir string
		stdout         io.Writer
		sourceDir      string
		sourcePath     string
		collErr        error
	)

	BeforeEach(func() {
		ctx = context.TODO()

		var err error
		sourceDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())

		destinationDir, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())

		stdout = gbytes.NewBuffer()
	})

	AfterEach(func() {
		Expect(os.RemoveAll(sourceDir)).To(Succeed())
		Expect(os.RemoveAll(destinationDir)).To(Succeed())
	})

	Context("copying a single file", func() {
		BeforeEach(func() {
			var err error
			sourcePath = filepath.Join(sourceDir, "file-to-collect")
			err = os.WriteFile(sourcePath, []byte("file-to-collect"), 0755)
			Expect(err).NotTo(HaveOccurred())
		})

		JustBeforeEach(func() {
			collErr = file.NewCollector(sourcePath, "destination_file").Run(ctx, destinationDir, stdout)
		})

		It("copies the specified file to the destination directory", func() {
			destinationFilePath := filepath.Join(destinationDir, "destination_file")

			Expect(collErr).NotTo(HaveOccurred())
			Expect(destinationFilePath).To(BeAnExistingFile())

			destinationFileContents, err := os.ReadFile(destinationFilePath)
			Expect(err).NotTo(HaveOccurred())
			Expect(destinationFileContents).To(Equal([]byte("file-to-collect")))
		})

		When("the copy fails", func() {
			BeforeEach(func() {
				sourcePath = "/i/do/not/exist"
			})

			It("returns an error", func() {
				Expect(collErr).To(MatchError(ContainSubstring("No such file or directory")))
			})
		})

		When("the destination path contains folders that don't exist", func() {
			BeforeEach(func() {
				destinationDir = filepath.Join(destinationDir, "foo", "bar")
			})

			It("creates the missing directories", func() {
				Expect(collErr).NotTo(HaveOccurred())

				Expect(destinationDir).To(BeADirectory())
				Expect(filepath.Join(destinationDir, "destination_file")).To(BeAnExistingFile())
			})
		})
	})

	Context("copying a directory", func() {
		var (
			files []string
		)

		BeforeEach(func() {
			files = []string{"foo", "bar", "sha"}

			sourcePath = filepath.Join(sourceDir, "garden")
			Expect(os.MkdirAll(sourcePath, 0755)).To(Succeed())

			for _, f := range files {
				err := os.WriteFile(filepath.Join(sourcePath, f), []byte("file-to-collect"), 0644)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		JustBeforeEach(func() {
			collErr = file.NewDirCollector(sourcePath, "").Run(ctx, destinationDir, stdout)
		})

		It("copies all source files to target", func() {
			Expect(collErr).NotTo(HaveOccurred())
			for _, f := range files {
				Expect(filepath.Join(destinationDir, "garden", f)).To(BeAnExistingFile())
			}
		})
	})

	Context("copying files with a glob pattern", func() {
		var (
			files []string
		)

		BeforeEach(func() {
			files = []string{"foo", "bar", "foobar"}

			sourcePath = filepath.Join(sourceDir, "garden")
			Expect(os.MkdirAll(sourcePath, 0755)).To(Succeed())

			for _, f := range files {
				err := os.WriteFile(filepath.Join(sourcePath, f), []byte("file-to-collect"), 0644)
				Expect(err).NotTo(HaveOccurred())
			}
		})

		JustBeforeEach(func() {
			collErr = file.NewCollector(filepath.Join(sourcePath, "foo*"), "target/").Run(ctx, destinationDir, stdout)
		})

		It("copies only foo and foobar to target", func() {
			Expect(collErr).NotTo(HaveOccurred())
			for _, f := range []string{"foo", "foobar"} {
				Expect(filepath.Join(destinationDir, "target", f)).To(BeAnExistingFile())
			}
			Expect(filepath.Join(destinationDir, "target", "bar")).ToNot(BeAnExistingFile())
		})
	})

})
