package gats98_test

import (
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var (
	gardenHost      string
	gardenPort      string
	gardenDebugPort string
	gardenClient    garden.Client

	testImage garden.ImageRef
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

func TestGats98(t *testing.T) {
	RegisterFailHandler(Fail)

	BeforeSuite(func() {
		if runtime.GOOS != "windows" {
			Skip("Skipping GATS98 as we are not running windows")
		}
		gardenRootfs, present := os.LookupEnv("WINDOWS_TEST_ROOTFS")
		if !present {
			fmt.Println("Must set $WINDOWS_TEST_ROOTFS")
			os.Exit(1)
		}
		testImage = garden.ImageRef{URI: gardenRootfs}
		host := os.Getenv("GDN_BIND_IP")
		if host == "" {
			host = "10.244.0.2"
		}
		gardenHost = resolveHost(host)
	})

	AfterSuite(func() {
		gexec.CleanupBuildArtifacts()
	})

	BeforeEach(func() {
		gardenPort = os.Getenv("GDN_BIND_PORT")
		if gardenPort == "" {
			gardenPort = "7777"
		}
		gardenDebugPort = os.Getenv("GDN_DEBUG_PORT")
		if gardenDebugPort == "" {
			gardenDebugPort = "17013"
		}
		gardenClient = client.New(connection.New("tcp", fmt.Sprintf("%s:%s", gardenHost, gardenPort)))
	})

	RunSpecs(t, "Gats98 Suite")
}

func runProcess(container garden.Container, processSpec garden.ProcessSpec) (exitCode int, stdout, stderr *gbytes.Buffer) {
	stdOut, stdErr := gbytes.NewBuffer(), gbytes.NewBuffer()
	proc, err := container.Run(
		processSpec,
		garden.ProcessIO{
			Stdout: io.MultiWriter(stdOut, GinkgoWriter),
			Stderr: io.MultiWriter(stdErr, GinkgoWriter),
		})
	Expect(err).NotTo(HaveOccurred())
	processExitCode, err := proc.Wait()
	Expect(err).NotTo(HaveOccurred())
	return processExitCode, stdOut, stdErr
}
