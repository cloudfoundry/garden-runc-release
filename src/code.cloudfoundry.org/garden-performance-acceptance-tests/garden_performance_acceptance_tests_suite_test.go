package garden_performance_acceptance_tests_test

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"

	"code.cloudfoundry.org/lager/v3"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var (
	gardenClient           garden.Client
	ignorePerfExpectations bool // allows us to report metrics even when an expectation fails
)

// We suspect that bosh powerdns lookups have a low success rate (less than
// 99%) and when it fails, we get an empty string IP address instead of an
// actual error.
// Therefore, we explicity look up the IP once at the start of the suite with
// retries to minimise flakes.
func resolveHost(host string) string {
	if net.ParseIP(host) != nil {
		return host
	}

	var ip net.IP
	Eventually(func() error {
		ips, err := net.LookupIP(host)
		if err != nil {
			return err
		}
		if len(ips) == 0 {
			return errors.New("0 IPs returned from DNS")
		}
		ip = ips[0]
		return nil
	}, time.Minute, time.Second*5).Should(Succeed())

	return ip.String()
}

var _ = BeforeSuite(func() {
	gardenHost := os.Getenv("GARDEN_ADDRESS")
	if gardenHost == "" {
		gardenHost = "127.0.0.1"
	}
	gardenHost = resolveHost(gardenHost)
	gardenPort := os.Getenv("GARDEN_PORT")
	if gardenPort == "" {
		gardenPort = "7777"
	}
	gardenClient = client.New(connection.New("tcp", fmt.Sprintf("%s:%s", gardenHost, gardenPort)))

	if os.Getenv("PREHEAT_SERVER") != "" {
		maxPreheat := 30000

		if max, err := strconv.Atoi(os.Getenv("PREHEAT_SERVER")); err == nil {
			maxPreheat = max
		}
		preheatServer(maxPreheat)
	}

	// ensure a 'clean' starting state
	cleanupContainers()
})

func TestGardenPerformanceAcceptanceTests(t *testing.T) {
	if os.Getenv("IGNORE_PERF_EXPECTATIONS") != "" {
		ignorePerfExpectations = true
	}

	logger := lager.NewLogger("garden-performance-acceptance-tests")
	logger.RegisterSink(lager.NewWriterSink(GinkgoWriter, lager.INFO))

	RegisterFailHandler(Fail)
	RunSpecs(t, "GardenPerformanceAcceptanceTests Suite")
}

func cleanupContainers() {
	containers, err := gardenClient.Containers(garden.Properties{})
	Expect(err).NotTo(HaveOccurred())

	count := len(containers)

	batchSize := count / 2

	batchA := containers[:batchSize]
	batchB := containers[batchSize:]

	waitGroup := sync.WaitGroup{}
	waitGroup.Add(2)

	go func() {
		defer GinkgoRecover()
		defer waitGroup.Done()
		for _, container := range batchA {
			Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
		}
	}()

	go func() {
		defer GinkgoRecover()
		defer waitGroup.Done()
		for _, container := range batchB {
			Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
		}
	}()

	waitGroup.Wait()
}

func Conditionally(expectation func(), condition bool) {
	if condition {
		expectation()
	}
}

// simulate a long-running guardian process via many, many Creates and Destroys
func preheatServer(total int) {
	batchSize := 10
	numGoroutines := 5
	count := 0
	countPerGoroutine := batchSize / numGoroutines

	waitGroup := sync.WaitGroup{}

	t := time.Now()
	fmt.Printf("Preheating the server (this will take a while)\n")

	for count < total {
		for i := 0; i < numGoroutines; i++ {
			waitGroup.Add(1)

			go func() {
				defer GinkgoRecover()
				defer waitGroup.Done()

				for j := 0; j < countPerGoroutine; j++ {
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
				}
			}()
		}

		waitGroup.Wait()

		cleanupContainers()
		count += batchSize
		fmt.Printf("\tBatch complete - %d/%d\n", count, total)
	}

	preheatDuration := time.Since(t)
	fmt.Printf("Preheating complete - took %s\n\n", preheatDuration)
}
