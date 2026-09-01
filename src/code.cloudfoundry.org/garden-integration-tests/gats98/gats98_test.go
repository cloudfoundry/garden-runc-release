package gats98_test

import (
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Windows", func() {
	var container garden.Container

	AfterEach(func() {
		if container != nil {
			// ignoring the error since it can return unknown handle error
			theContainer, _ := gardenClient.Lookup(container.Handle())

			if theContainer != nil {
				Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
			}
		}
	})

	BeforeEach(func() {
		var err error
		container, err = gardenClient.Create(garden.ContainerSpec{
			Handle:     "",
			Image:      testImage,
			Privileged: false,
			Properties: garden.Properties{},
			Env:        []string{},
			Limits: garden.Limits{
				CPU: garden.CPULimits{
					Weight: 1234,
				},
			},
			Network: "",
		})
		Expect(err).NotTo(HaveOccurred())
	})

	It("can run a process in a container", func() {
		exitCode, stdout, _ := runProcess(container, garden.ProcessSpec{
			User: "",
			Path: "cmd.exe",
			Args: []string{"/C", `echo hello`},
		})

		Expect(exitCode).To(Equal(0))
		Expect(stdout).To(gbytes.Say("hello"))
	})

	// We verify that the container.Age attribute is filled
	// since winc does not provide this.
	// The default would otherwise be zero time, aka 0001-01-01 00:00:00
	It("can get the age of a container", func() {
		metrics, err := container.Metrics()
		Expect(err).NotTo(HaveOccurred())

		Expect(metrics.Age).To(BeNumerically(">", 0))
		Expect(metrics.Age).To(BeNumerically("<", time.Second))
	})
})
