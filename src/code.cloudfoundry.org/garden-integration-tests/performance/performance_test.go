package performance_test

import (
	"compress/gzip"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	wavefront "github.com/wavefronthq/wavefront-sdk-go/senders"
)

const containerPrefix = "concurrent-create-handle"

func mustGetEnv(key string) string {
	val, ok := os.LookupEnv(key)
	Expect(ok).To(BeTrue(), fmt.Sprintf("%q env var not set", key))
	return val
}

var _ = Describe("performance", func() {
	var (
		wfSender wavefront.Sender
	)

	BeforeEach(func() {
		//https://github.com/wavefrontHQ/wavefront-sdk-go/blob/master/senders/client_factory_test.go#L88
		//URL should have token embeded
		wfURL := mustGetEnv("WAVEFRONT_URL")

		var err error
		wfSender, err = wavefront.NewSender(wfURL)
		Expect(err).NotTo(HaveOccurred())
	})

	emitMetric := func(metricName string, metricVal float64, timestamp int64, source string) {
		Expect(wfSender.SendMetric(metricName, metricVal, timestamp, source, nil)).To(Succeed())
		Expect(wfSender.Flush()).To(Succeed())
	}

	createAndStream := func(index int, b Benchmarker, archive string) {
		var handle string
		var ctr garden.Container
		var err error

		b.Time(fmt.Sprintf("stream-%d", index), func() {
			creationTime := b.Time(fmt.Sprintf("create-%d", index), func() {
				By("creating container " + strconv.Itoa(index))
				ctr, err = gardenClient.Create(garden.ContainerSpec{
					Limits: garden.Limits{
						Disk: garden.DiskLimits{ByteHard: 2 * 1024 * 1024 * 1024},
					},
					Privileged: true,
				})
				Expect(err).ToNot(HaveOccurred())
				handle = ctr.Handle()
				By("done creating container " + strconv.Itoa(index))
			})
			now := time.Now()
			emitMetric("garden.container-creation-time", float64(creationTime), now.Unix(), os.Getenv("WAVEFRONT_METRIC_PREFIX"))
		})

		By("starting stream in to container " + handle)

		streamin(ctr, archive)

		By("succefully streamed in to container " + handle)

		b.Time(fmt.Sprintf("delete-%d", index), func() {
			By("destroying container " + handle)
			Expect(gardenClient.Destroy(handle)).To(Succeed())
			By("successfully destroyed container " + handle)
		})
	}

	AfterEach(func() {
		wfSender.Close()
	})

	JustBeforeEach(func() {
		warmUp(gardenClient)
	})

	Measure("multiple concurrent creates", func(b Benchmarker) {
		concurrencyLevel := 5
		handles := []string{}

		b.Time("concurrent creations", func() {
			wg := sync.WaitGroup{}

			for i := 0; i < concurrencyLevel; i++ {
				wg.Add(1)
				h := fmt.Sprintf("%s-%d-%d", containerPrefix, GinkgoParallelProcess(), i)
				handles = append(handles, h)

				go func(index int, handle string) {
					defer wg.Done()
					defer GinkgoRecover()

					b.Time(fmt.Sprintf("create-%d", index), func() {
						_, err := gardenClient.Create(garden.ContainerSpec{Handle: handle})
						Expect(err).ToNot(HaveOccurred())
					})
				}(i, h)
			}

			wg.Wait()
		})

		b.Time("destroy", func() {
			for _, handle := range handles {
				b.Time(fmt.Sprintf("destroy-%s", handle), func() {
					Expect(gardenClient.Destroy(handle)).To(Succeed())
				})
			}

			// ensure all containers are actually destroyed
			Eventually(func() error {
				for _, handle := range handles {
					_, err := gardenClient.Lookup(handle)
					if err == nil {
						return fmt.Errorf("container %q exists but it should've been destroyed", handle)
					}
				}
				return nil
			}).ShouldNot(HaveOccurred())
		})
	}, 50)

	Measure("serial creation of containers with disk quotas", func(b Benchmarker) {
		handles := []string{}

		for i := 0; i < 50; i++ {
			b.Time(fmt.Sprintf("create-%d", i), func() {
				containerSpec := garden.ContainerSpec{
					Handle: fmt.Sprintf("container-%d", i),
					Limits: garden.Limits{
						Disk: garden.DiskLimits{ByteHard: 2 * 1024 * 1024 * 1024},
					},
				}
				container, err := gardenClient.Create(containerSpec)
				Expect(err).NotTo(HaveOccurred())
				handles = append(handles, container.Handle())
			})
		}

		for _, handle := range handles {
			Expect(gardenClient.Destroy(handle)).To(Succeed())
		}
	}, 10)

	Context("streaming custom tgz file", func() {
		const archive string = "file.tgz"

		JustBeforeEach(func() {
			// create a 17M tgz file
			Expect(exec.Command("dd", "if=/dev/urandom", "of=file", "bs=1048576", "count=17").Run()).To(Succeed())
			Expect(exec.Command("/bin/bash", "-c", fmt.Sprintf("tar cvzf %s file", archive)).Run()).To(Succeed())

			Expect(archive).To(BeARegularFile())
		})

		AfterEach(func() {
			os.Remove("file")
			os.Remove(archive)
		})

		Measure("stream bytes in", func(b Benchmarker) {
			concurrenyLevel := 5
			By("starting")

			b.Time("concurrent streamings", func() {
				wg := sync.WaitGroup{}

				for i := 0; i < concurrenyLevel; i++ {
					wg.Add(1)

					go func(index int) {
						defer wg.Done()
						defer GinkgoRecover()

						// do it twice in a row to increase likelihood of overlaps
						createAndStream(index, b, archive)
						createAndStream(index, b, archive)
					}(i)
				}

				wg.Wait()
			})
		}, 10)
	})

	Describe("streaming", func() {
		BeforeEach(func() {
			rootfs = "docker:///cloudfoundry/garden-rootfs"
		})

		Measure("it should stream stdout and stderr efficiently", func(b Benchmarker) {
			b.Time("(baseline) streaming 50M of stdout to /dev/null", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				_, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", "tr '\\0' 'a' < /dev/zero | dd count=50 bs=1M of=/dev/null; echo done"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout, "2s").Should(gbytes.Say("done\n"))
			})

			time := b.Time("streaming 50M of data via garden", func() {
				stdout := gbytes.NewBuffer()
				stderr := gbytes.NewBuffer()

				_, err := container.Run(garden.ProcessSpec{
					User: "alice",
					Path: "sh",
					Args: []string{"-c", "tr '\\0' 'a' < /dev/zero | dd count=50 bs=1M; echo done"},
				}, garden.ProcessIO{
					Stdout: stdout,
					Stderr: stderr,
				})
				Expect(err).ToNot(HaveOccurred())

				Eventually(stdout, "10s").Should(gbytes.Say("done\n"))
			})

			Expect(time.Seconds()).To(BeNumerically("<", 3))
		}, 10)
	})

	Describe("a process inside a container", func() {
		BeforeEach(func() {
			rootfs = "docker:///cloudfoundry/garden-fuse"
		})

		Measure("starting lots of processes", func(b Benchmarker) {
			b.Time("end to end time", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "bash",
					Args: []string{"-c", `
					for i in {1..1000}
					do
						/bin/echo hi > /dev/null
					done
				`},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: GinkgoWriter,
				})
				Expect(err).NotTo(HaveOccurred())
				Expect(process.Wait()).To(Equal(0))
			})

			// TODO add expectations to avoid regression
		}, 20)

		Measure("running a calculation", func(b Benchmarker) {
			stderr := gbytes.NewBuffer()
			b.Time("end to end time", func() {
				process, err := container.Run(garden.ProcessSpec{
					User: "root",
					Path: "bash",
					Args: []string{
						"-c",
						`time echo "scale=1000; a(1)*4" | bc -l`,
					},
				}, garden.ProcessIO{
					Stdout: GinkgoWriter,
					Stderr: io.MultiWriter(stderr, GinkgoWriter),
				})

				Expect(err).NotTo(HaveOccurred())
				Expect(process.Wait()).To(Equal(0))
			})

			timeTaken := func(lines string) string {
				for _, line := range strings.Split(lines, "\n") {
					cols := strings.Fields(line)
					if len(cols) < 2 {
						continue
					}
					if cols[0] == "user" {
						return cols[1]
					}
				}
				return "error!"
			}

			dur, err := time.ParseDuration(timeTaken(string(stderr.Contents())))
			Expect(err).NotTo(HaveOccurred())

			b.RecordValue("time in calculation", dur.Seconds())

			// Once we have a good baseline...
			//Expect(timed).To(BeNumerically(",", ???))
			//Expect(b.Seconds()).To(BeNumerically(",", ???))
		}, 20)
	})

	Measure("BulkNetOut", func(b Benchmarker) {
		b.Time("3000 rules", func() {
			rules := make([]garden.NetOutRule, 0)

			for i := 0; i < 3000; i++ {
				rules = append(rules, garden.NetOutRule{
					Protocol: garden.ProtocolTCP,
					Networks: []garden.IPRange{garden.IPRangeFromIP(net.ParseIP("8.8.8.8"))},
					Ports:    []garden.PortRange{garden.PortRangeFromPort(uint16(i))},
				})
			}

			container, err := gardenClient.Create(garden.ContainerSpec{})
			Expect(err).ToNot(HaveOccurred())

			Expect(container.BulkNetOut(rules)).To(Succeed())

			Expect(gardenClient.Destroy(container.Handle())).To(Succeed())
		})
	}, 5)
})

func warmUp(gardenClient garden.Client) {
	ctr, err := gardenClient.Create(garden.ContainerSpec{})
	Expect(err).ToNot(HaveOccurred())
	Expect(gardenClient.Destroy(ctr.Handle())).To(Succeed())
}

func streamin(ctr garden.Container, archive string) {
	for i := 0; i < 20; i++ {
		By(fmt.Sprintf("preparing stream %d for handle %s", i, ctr.Handle()))
		// Stream in a tar file to ctr
		var tarStream io.Reader

		pwd, err := os.Getwd()
		Expect(err).ToNot(HaveOccurred())
		tgzPath := path.Join(pwd, archive)
		tgz, err := os.Open(tgzPath)
		Expect(err).ToNot(HaveOccurred())
		tarStream, err = gzip.NewReader(tgz)
		Expect(err).ToNot(HaveOccurred())

		By(fmt.Sprintf("starting stream %d for handle: %s", i, ctr.Handle()))
		Expect(ctr.StreamIn(garden.StreamInSpec{
			User:      "root",
			Path:      fmt.Sprintf("/root/stream-file-%d", i),
			TarStream: tarStream,
		})).To(Succeed())
		By(fmt.Sprintf("stream %d done for handle: %s", i, ctr.Handle()))

		tgz.Close()
	}
}
