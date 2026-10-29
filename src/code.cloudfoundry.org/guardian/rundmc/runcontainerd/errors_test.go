package runcontainerd_test

import (
	"errors"
	"fmt"

	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("not-found errors", func() {
	type notFoundError interface {
		IsNotFound() bool
	}

	DescribeTable("mark definitive absence, also when wrapped",
		func(err error) {
			var notFound notFoundError
			Expect(errors.As(fmt.Errorf("context: %w", err), &notFound)).To(BeTrue())
			Expect(notFound.IsNotFound()).To(BeTrue())
		},
		Entry("container", runcontainerd.ContainerNotFoundError{Handle: "some-handle"}),
		Entry("task", runcontainerd.TaskNotFoundError{Handle: "some-handle"}),
	)
})
