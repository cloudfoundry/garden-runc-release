package command_test

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"

	"code.cloudfoundry.org/dontpanic/collectors/command"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Command Output Collector", func() {
	var (
		ctx       context.Context
		dstPath   string
		stdout    io.Writer
		collector command.Collector

		err error
	)

	BeforeEach(func() {
		ctx = context.Background()
		dstPath, err = os.MkdirTemp("", "")
		Expect(err).NotTo(HaveOccurred())
		stdout = gbytes.NewBuffer()
	})

	Describe("collect output to a file", func() {
		var (
			filename string
			cmd      string
		)

		BeforeEach(func() {
			filename = "hello"
		})

		JustBeforeEach(func() {
			collector = command.NewCollector(cmd, filename)
			err = collector.Run(ctx, dstPath, stdout)
		})

		When("cmd is a simple executable", func() {
			BeforeEach(func() {
				cmd = "echo hello world"
			})

			It("returns the byte output", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(gbytes.Say("hello world\n"))

				fileContents, err := os.ReadFile(filepath.Join(dstPath, filename))
				Expect(err).NotTo(HaveOccurred())
				Expect(fileContents).To(Equal([]byte("hello world\n")))
			})
		})

		When("cmd is a pipeline", func() {
			BeforeEach(func() {
				cmd = "seq 2 10 | wc -l"
			})

			It("executes the pipeline", func() {
				Expect(err).NotTo(HaveOccurred())
				Expect(stdout).To(gbytes.Say("9"))

				fileContents, err := os.ReadFile(filepath.Join(dstPath, filename))
				Expect(err).NotTo(HaveOccurred())
				Expect(strings.Trim(string(fileContents), " ")).To(Equal("9\n"))
			})
		})

		When("command fails and has stdout and stderr", func() {
			BeforeEach(func() {
				cmd = "echo foo; echo bar >&2; exit 1"
			})

			It("returns error containing bar", func() {
				Expect(err).To(MatchError(ContainSubstring("bar")))
				Expect(filepath.Join(dstPath, filename)).NotTo(BeAnExistingFile())
				Expect(stdout).NotTo(gbytes.Say("foo"))
			})
		})
	})

	Describe("discard collector", func() {
		JustBeforeEach(func() {
			collector = command.NewDiscardCollector("echo hello world")
			err = collector.Run(ctx, dstPath, stdout)
		})

		It("does not write to a file", func() {
			filesInDstPath, err := os.ReadDir(dstPath)
			Expect(err).NotTo(HaveOccurred())
			Expect(filesInDstPath).To(BeEmpty())
		})
	})
})
