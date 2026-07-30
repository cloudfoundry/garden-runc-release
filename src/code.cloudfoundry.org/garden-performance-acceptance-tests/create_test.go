package garden_performance_acceptance_tests_test

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"sync"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-performance-acceptance-tests/measurements"
	"code.cloudfoundry.org/garden-performance-acceptance-tests/reporter"
)

const (
	ContainerCreation = "ContainerCreation"
)

var _ = Describe("Create", func() {
	AfterEach(func() {
		cleanupContainers()
	})

	Measure("must take less than 1.5 seconds for each container", func(b Benchmarker) {
		waitGroup := sync.WaitGroup{}
		measurementsChan := make(chan float64, 250)

		for i := 0; i < 5; i++ {
			waitGroup.Add(1)

			go func() {
				defer waitGroup.Done()
				defer GinkgoRecover()

				for j := 0; j < 40; j++ {
					createTime := b.Time("create", func() {
						_, err := gardenClient.Create(
							garden.ContainerSpec{
								Limits: garden.Limits{
									Disk: garden.DiskLimits{
										ByteHard: 1024 * 1024,
										Scope:    garden.DiskLimitScopeExclusive,
									},
								},
							},
						)
						Expect(err).NotTo(HaveOccurred())
					},
						reporter.ReporterInfo{
							MetricName: ContainerCreation,
						},
					)
					measurementsChan <- createTime.Seconds()
				}

			}()
		}

		waitGroup.Wait()
		close(measurementsChan)
		i := 0
		createTimes := measurements.Measurements{}
		for t := range measurementsChan {
			fmt.Printf("measurement %d: %f\n", i, t)
			createTimes = append(createTimes, t)
			i++
		}

		averageCreateTime, err := createTimes.Average()
		Expect(err).NotTo(HaveOccurred())

		Conditionally(func() {
			Expect(averageCreateTime).To(BeNumerically("<", 1.5))
		}, !ignorePerfExpectations)
	}, 1)
})
