package garden_integration_tests_test

import (
	"os"
	"runtime"

	"code.cloudfoundry.org/garden"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
)

var _ = Describe("Rootfses", func() {
	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("pending for windows")
		}
	})

	Context("when the rootfs path is a private azure image URL", func() {
		BeforeEach(func() {
			imageRef.URI = "docker://gnome.azurecr.io/alpine#3.7"
			imageRef.Username = os.Getenv("AZURE_REGISTRY_USERNAME")
			imageRef.Password = os.Getenv("AZURE_REGISTRY_PASSWORD")
			if imageRef.Username == "" || imageRef.Password == "" {
				Skip("Registry username or password not provided")
			}
		})

		It("should succeed", func() {
			Expect(containerCreateErr).NotTo(HaveOccurred())
		})
	})

	Context("when the rootfs path is a docker image URL", func() {
		Context("and the image specifies $PATH", func() {
			BeforeEach(func() {
				imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
				skipIfWoot("Groot does not place environemnt variables in the bundle spec yet")
			})

			It("$PATH is taken from the docker image", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "echo $PATH"},
				})

				Expect(stdout).To(gbytes.Say("/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/bin:/from-dockerfile"))
			})

			It("$TEST is taken from the docker image", func() {
				stdout := runForStdout(container, garden.ProcessSpec{
					User: "root",
					Path: "/bin/sh",
					Args: []string{"-c", "echo $TEST"},
				})

				Expect(stdout).To(gbytes.Say("second-test-from-dockerfile:test-from-dockerfile"))
			})

		})

		Context("and the image is private", func() {
			BeforeEach(func() {
				imageRef.URI = "docker:///cloudfoundry/garden-private-image-test"
				imageRef.Username = os.Getenv("DOCKER_REGISTRY_USERNAME")
				imageRef.Password = os.Getenv("DOCKER_REGISTRY_PASSWORD")
				if imageRef.Username == "" || imageRef.Password == "" {
					Skip("Registry username or password not provided")
				}
				assertContainerCreate = false
			})

			It("successfully pulls the image", func() {
				Expect(containerCreateErr).ToNot(HaveOccurred())
			})

			Context("but the credentials are incorrect", func() {
				BeforeEach(func() {
					imageRef.Username = ""
					imageRef.Password = ""
				})

				It("fails", func() {
					Expect(containerCreateErr).NotTo(Succeed())
				})
			})
		})
	})

	Context("and the Docker image contains opaque whiteouts", func() {
		BeforeEach(func() {
			imageRef.URI = "docker:///cloudfoundry/garden-rootfs"
		})

		It("handles them correctly", func() {
			exitCode, _, _ := runProcess(container,
				garden.ProcessSpec{
					Path: "ls",
					Args: []string{"/test/foo"},
				})
			Expect(exitCode).To(Equal(0))
		})
	})
})
