package commandrunner_test

import (
	"context"
	"time"

	"code.cloudfoundry.org/dontpanic/commandrunner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Commandrunner", func() {
	var (
		cmdRunner commandrunner.CommandRunner
		ctx       context.Context
		runOutput []byte
		runErr    error
		command   string
		args      []string
	)
	BeforeEach(func() {
		ctx = context.TODO()
		cmdRunner = commandrunner.CommandRunner{}
		command = "echo"
		args = []string{"-n", "hello"}
	})

	JustBeforeEach(func() {
		runOutput, runErr = cmdRunner.Run(ctx, command, args...)
	})

	It("calls the given command and returns the output", func() {
		Expect(runErr).NotTo(HaveOccurred())
		Expect(string(runOutput)).To(Equal("hello"))
	})

	Context("when the command fails", func() {
		BeforeEach(func() {
			command = "cat"
			args = []string{"/does/not/exist"}
		})

		It("we get the error back", func() {
			Expect(runErr.Error()).To(ContainSubstring("No such file or directory"))
		})

	})

	Context("when the command exceeds the deadline", func() {
		var cancel context.CancelFunc
		BeforeEach(func() {
			ctx, cancel = context.WithTimeout(context.Background(), time.Nanosecond)
		})

		AfterEach(func() {
			cancel()
		})

		It("returns an error", func() {
			Expect(runErr).To(Equal(context.DeadlineExceeded))
		})
	})

})
