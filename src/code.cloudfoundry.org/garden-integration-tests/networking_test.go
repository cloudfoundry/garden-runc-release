package garden_integration_tests_test

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

const googleDNSIP = "8.8.8.8"

var _ = Describe("Networking", func() {
	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
	})

	It("can be contacted after a NetIn", func() {
		_, err := container.Run(garden.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", "echo hallo | nc -l -p 8080"},
			User: "root",
		}, garden.ProcessIO{
			Stdout: GinkgoWriter,
			Stderr: GinkgoWriter,
		})
		Expect(err).ToNot(HaveOccurred())

		hostPort, _, err := container.NetIn(0, 8080)
		Expect(err).ToNot(HaveOccurred())

		gardenHostname := strings.Split(gardenHost, ":")[0]
		Eventually(func() error {
			nc, err := gexec.Start(exec.Command("nc", gardenHostname, fmt.Sprintf("%d", hostPort)), GinkgoWriter, GinkgoWriter)
			if err != nil {
				Eventually(nc).Should(gbytes.Say("hallo"))
			}

			return err
		}).ShouldNot(HaveOccurred())
	})

	It("container root can overwrite /etc/hosts", func() {
		exitCode, _, _ := runProcess(container, garden.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", "echo NONSENSE > /etc/hosts"},
			User: "root",
		})

		Expect(exitCode).To(Equal(0))
	})

	It("container root can overwrite /etc/resolv.conf", func() {
		exitCode, _, _ := runProcess(container, garden.ProcessSpec{
			Path: "sh",
			Args: []string{"-c", "echo NONSENSE > /etc/resolv.conf"},
			User: "root",
		})

		Expect(exitCode).To(Equal(0))
	})

	Describe("running as a user other than container root", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		It("non-container-root can't overwrite /etc/hosts", func() {
			exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo NONSENSE > /etc/hosts"},
				User: "alice",
			})
			Expect(exitCode).To(Equal(1))
			Expect(stderr).To(gbytes.Say("Permission denied"))
		})

		It("non-container-root can't overwrite /etc/resolv.conf", func() {
			exitCode, _, stderr := runProcess(container, garden.ProcessSpec{
				Path: "sh",
				Args: []string{"-c", "echo NONSENSE > /etc/resolv.conf"},
				User: "alice",
			})
			Expect(exitCode).To(Equal(1))
			Expect(stderr).To(gbytes.Say("Permission denied"))
		})
	})

	Describe("domain name resolution", func() {
		tryPing := func(address string) string {
			var output bytes.Buffer

			GinkgoWriter.TeeTo(&output)
			proc, err := container.Run(garden.ProcessSpec{
				Path: "ping",
				Args: []string{"-W", "2", "-c", "1", address},
			}, garden.ProcessIO{
				Stdout: GinkgoWriter,
				Stderr: GinkgoWriter,
			})
			Expect(err).NotTo(HaveOccurred())

			pingExitCh := make(chan struct{})
			go func(pingProc garden.Process, exitCh chan<- struct{}) {
				defer GinkgoRecover()
				defer close(exitCh)
				_, err := pingProc.Wait()
				Expect(err).NotTo(HaveOccurred())
			}(proc, pingExitCh)

			select {
			case <-pingExitCh:
				return output.String()
			case <-time.After(time.Second * 2):
				close(pingExitCh)
				return "timed out after 2 seconds"
			}
		}

		itCanResolve := func(domainName string) {
			EventuallyWithOffset(1, func() string {
				return tryPing(domainName)
			}).Should(ContainSubstring("1 packets transmitted, 1 packets received"))
		}

		It("can resolve localhost", func() {
			itCanResolve("localhost")
		})

		It("can resolve its hostname", func() {
			itCanResolve(container.Handle())
		})

		Context("when the rootFS contains /etc/resolv.conf", func() {
			BeforeEach(func() {
				imageRef.URI = "docker:///debian#jessie-20180312"
			})

			It("can resolve domain names", func() {
				itCanResolve("www.example.com")
			})
		})

		Context("when the rootFS doesn't contain /etc/hosts or /etc/resolv.conf", func() {
			BeforeEach(func() {
				imageRef.URI = "docker:///busybox#1.37.0"
			})

			It("can still resolve domain names because garden modifies /etc/resolv.conf", func() {
				itCanResolve("www.example.com")
			})

			It("can still resolve its hostname because garden modifies /etc/hosts", func() {
				itCanResolve(container.Handle())
			})
		})
	})

	Describe("subnet support", func() {
		BeforeEach(func() {
			networkSpec = fmt.Sprintf("192.168.%d.0/24", 12+GinkgoParallelProcess())
		})

		Context("when destroying other containers on the same subnet", func() {
			It("should continue to route traffic successfully", func() {
				var (
					err            error
					otherContainer garden.Container
				)

				for i := 0; i < 5; i++ {
					otherContainer, err = gardenClient.Create(garden.ContainerSpec{
						Network: networkSpec,
					})
					Expect(err).NotTo(HaveOccurred())

					Expect(gardenClient.Destroy(otherContainer.Handle())).To(Succeed())
					err := checkConnection(container, googleDNSIP, 53)
					if err != nil {
						checkPing(container, googleDNSIP)
					}
					Expect(err).NotTo(HaveOccurred())
				}
			})
		})

		Context("when creating a container in a previously used subnet", func() {
			var newContainer garden.Container

			JustBeforeEach(func() {
				var err error

				Expect(gardenClient.Destroy(container.Handle())).To(Succeed())

				newContainer, err = gardenClient.Create(garden.ContainerSpec{
					Network: networkSpec,
				})
				Expect(err).NotTo(HaveOccurred())
			})

			AfterEach(func() {
				Expect(gardenClient.Destroy(newContainer.Handle())).To(Succeed())
			})

			It("should continue to route traffic successfully", func() {
				Expect(checkConnection(newContainer, googleDNSIP, 53)).To(Succeed())
			})
		})
	})
})

func checkConnection(container garden.Container, ip string, port int) error {
	process, err := container.Run(garden.ProcessSpec{
		User: "root",
		Path: "sh",
		Args: []string{"-c", fmt.Sprintf("echo hello | nc -w3 %s %d", ip, port)},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	if err != nil {
		return err
	}

	exitCode, err := process.Wait()
	if err != nil {
		return err
	}

	if exitCode == 0 {
		return nil
	} else {
		return fmt.Errorf("Request failed. Process exited with code %d", exitCode)
	}
}

func checkPing(container garden.Container, ip string) error {
	p, err := container.Run(garden.ProcessSpec{
		User: "root",
		Path: "ping",
		Args: []string{"-c", "10", "-W", "1", ip},
	}, garden.ProcessIO{Stdout: GinkgoWriter, Stderr: GinkgoWriter})
	p.Wait()

	return err
}
