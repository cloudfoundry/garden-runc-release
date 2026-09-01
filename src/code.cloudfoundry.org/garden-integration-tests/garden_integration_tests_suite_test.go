package garden_integration_tests_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden-integration-tests/testhelpers"
	"code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
)

var (
	gardenHost            string
	gardenPort            string
	gardenDebugPort       string
	gardenRootfs          string
	gardenClient          garden.Client
	container             garden.Container
	containerStartUsage   uint64
	containerCreateErr    error
	assertContainerCreate bool

	handle              string
	imageRef            garden.ImageRef
	networkSpec         string
	privilegedContainer bool
	properties          garden.Properties
	limits              garden.Limits
	env                 []string

	consumeBin string

	limitsTestURI                string
	limitsTestContainerImageSize uint64 // Obtained by summing the values in <groot-image-store>\layers\<layer-id>\size
)

var _ = SynchronizedBeforeSuite(func() []byte {

	host := os.Getenv("GDN_BIND_IP")
	if host == "" {
		host = "10.244.0.2"
	}

	rootfs, exists := os.LookupEnv("GARDEN_TEST_ROOTFS")
	ExpectWithOffset(1, exists).To(BeTrue(), "Set GARDEN_TEST_ROOTFS Env variable")

	binary := ""
	if runtime.GOOS == "windows" {
		var err error
		binary, err = gexec.Build("code.cloudfoundry.org/garden-integration-tests/plugins/consume-mem")
		Expect(err).ToNot(HaveOccurred())
	}

	limitsURI, exists := os.LookupEnv("LIMITS_TEST_URI")
	ExpectWithOffset(1, exists).To(BeTrue(), "Set LIMITS_TEST_URI Env variable")

	testData := make(map[string]interface{})
	testData["gardenHost"] = host
	testData["gardenRootfs"] = rootfs
	testData["consumeBin"] = binary
	testData["limitsTestUri"] = limitsURI

	json, err := json.Marshal(testData)
	Expect(err).NotTo(HaveOccurred())

	return json
}, func(jsonBytes []byte) {
	testData := make(map[string]interface{})
	Expect(json.Unmarshal(jsonBytes, &testData)).To(Succeed())

	gardenHost = testData["gardenHost"].(string)
	gardenRootfs = testData["gardenRootfs"].(string)
	consumeBin = testData["consumeBin"].(string)
	limitsTestURI = testData["limitsTestUri"].(string)
	limitsTestContainerImageSize = 4562899158 //Used only in windows tests
})

func TestGardenIntegrationTests(t *testing.T) {
	RegisterFailHandler(Fail)

	SetDefaultEventuallyTimeout(15 * time.Second)

	AfterSuite(func() {
		gexec.CleanupBuildArtifacts()
	})

	BeforeEach(func() {
		assertContainerCreate = true
		handle = ""
		imageRef = garden.ImageRef{}
		networkSpec = ""
		privilegedContainer = false
		properties = garden.Properties{}
		limits = garden.Limits{}
		env = []string{}
		gardenPort = os.Getenv("GDN_BIND_PORT")
		if gardenPort == "" {
			gardenPort = "7777"
		}
		gardenDebugPort = os.Getenv("GDN_DEBUG_PORT")
		if gardenDebugPort == "" {
			gardenDebugPort = "17013"
		}
		retryingConnection := testhelpers.RetryingConnection{Connection: connection.New("tcp", fmt.Sprintf("%s:%s", gardenHost, gardenPort))}
		gardenClient = client.New(&retryingConnection)
	})

	JustBeforeEach(func() {
		container, containerCreateErr = gardenClient.Create(garden.ContainerSpec{
			Handle:     handle,
			Image:      imageRef,
			Privileged: privilegedContainer,
			Properties: properties,
			Env:        env,
			Limits:     limits,
			Network:    networkSpec,
		})

		if container != nil {
			containerStartUsage = getContainerUsage(container.Handle())
			fmt.Fprintf(GinkgoWriter, "Container handle: %s\n", container.Handle())
		}

		if assertContainerCreate {
			Expect(containerCreateErr).ToNot(HaveOccurred())
		}
	})

	AfterEach(func() {
		if container != nil {
			Expect(destroyContainer(container)).To(Succeed())
		}
	})

	RunSpecs(t, "GardenIntegrationTests Suite")
}

func destroyContainer(c garden.Container) error {
	// ignoring the error since it can return unknown handle error
	theContainer, _ := gardenClient.Lookup(c.Handle())

	if theContainer != nil {
		return gardenClient.Destroy(theContainer.Handle())
	}

	return nil
}

func getContainerHandles() []string {
	containers, err := gardenClient.Containers(nil)
	Expect(err).ToNot(HaveOccurred())

	handles := make([]string, len(containers))
	for i, c := range containers {
		handles[i] = c.Handle()
	}

	return handles
}

func createUser(container garden.Container, username string) {
	if container == nil {
		return
	}

	var spec garden.ProcessSpec
	if runtime.GOOS == "windows" {
		spec = garden.ProcessSpec{
			User: "",
			Path: "cmd.exe",
			Args: []string{"/c", fmt.Sprintf("net user %s /ADD /passwordreq:no && runas /user:%s whoami", username, username)},
		}
	} else {
		spec = garden.ProcessSpec{
			User: "root",
			Path: "sh",
			Args: []string{"-c", fmt.Sprintf("id -u %s || adduser -D %s", username, username)},
		}
	}

	process, err := container.Run(spec, garden.ProcessIO{
		Stdout: GinkgoWriter,
		Stderr: GinkgoWriter,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0))
}

func getKernelVersion() (int, int) {
	ctr, err := gardenClient.Create(garden.ContainerSpec{})
	Expect(err).NotTo(HaveOccurred())
	defer gardenClient.Destroy(ctr.Handle())

	var outBytes bytes.Buffer
	process, err := ctr.Run(garden.ProcessSpec{
		User: "root",
		Path: "uname",
		Args: []string{"-r"},
	}, garden.ProcessIO{
		Stdout: &outBytes,
	})
	Expect(err).ToNot(HaveOccurred())
	Expect(process.Wait()).To(Equal(0))

	vSplit := strings.Split(outBytes.String(), ".")
	major, err := strconv.Atoi(vSplit[0])
	Expect(err).NotTo(HaveOccurred())
	minor, err := strconv.Atoi(vSplit[1])
	Expect(err).NotTo(HaveOccurred())

	return major, minor
}

func skipIfWoot(reason string) {
	if woot() {
		Skip("Skipping this test because I am WOOT: " + reason)
	}
}

func woot() bool {
	return os.Getenv("WOOT") != ""
}

func skipIfShed() {
	if shed() {
		Skip("Skipping this test - not applicable to shed")
	}
}

func shed() bool {
	return os.Getenv("SHED") != ""
}

func skipIfContainerdForProcesses() {
	if isContainerdForProcesses() {
		Skip("Skipping because containerd support for processes is enabled")
	}
}

func isContainerdForProcesses() bool {
	return os.Getenv("CONTAINERD_FOR_PROCESSES_ENABLED") != "false"
}

func setPrivileged() {
	privilegedContainer = true
}

func runProcessWithIO(container garden.Container, processSpec garden.ProcessSpec, pio garden.ProcessIO) int {
	proc, err := container.Run(processSpec, pio)
	Expect(err).NotTo(HaveOccurred())
	processExitCode, err := proc.Wait()
	Expect(err).NotTo(HaveOccurred())
	return processExitCode
}

func runProcess(container garden.Container, processSpec garden.ProcessSpec) (exitCode int, stdout, stderr *gbytes.Buffer) {
	stdout, stderr = gbytes.NewBuffer(), gbytes.NewBuffer()
	pio := garden.ProcessIO{
		Stdout: io.MultiWriter(stdout, GinkgoWriter),
		Stderr: io.MultiWriter(stderr, GinkgoWriter),
	}
	exitCode = runProcessWithIO(container, processSpec, pio)
	return
}

func runForStdout(container garden.Container, processSpec garden.ProcessSpec) (stdout *gbytes.Buffer) {
	exitCode, stdout, _ := runProcess(container, processSpec)
	ExpectWithOffset(1, exitCode).To(Equal(0))
	return stdout
}

func getContainerUsage(handle string) uint64 {
	if runtime.GOOS != "windows" {
		return 0
	}
	// Get the volume path
	winc_binary, exists := os.LookupEnv("WINC_BINARY")
	ExpectWithOffset(1, exists).To(BeTrue(), "Set WINC_BINARY Env variable")

	cmd := exec.Command(winc_binary, "state", handle)
	cmdOut := new(bytes.Buffer)
	cmd.Stdout = cmdOut
	err := cmd.Run()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	// Get the pid
	type containerState struct {
		Pid int `json:"pid"`
	}
	var cs containerState
	err = json.Unmarshal(cmdOut.Bytes(), &cs)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())

	// Get the amount of disk in use
	path := fmt.Sprintf("C:\\proc\\%d\\root", cs.Pid)

	cmd = exec.Command("powershell", fmt.Sprintf("(Get-FSRMQuota %s).Usage", path))
	cmdOut = new(bytes.Buffer)
	cmd.Stdout = cmdOut
	err = cmd.Run()
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	usage := strings.TrimSpace(cmdOut.String())
	if usage == "" {
		usage = "0"
	}

	uintUsage, err := strconv.ParseUint(usage, 10, 64)
	ExpectWithOffset(1, err).ToNot(HaveOccurred())
	return uintUsage
}

func httpGet(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}

	return string(body), nil
}
