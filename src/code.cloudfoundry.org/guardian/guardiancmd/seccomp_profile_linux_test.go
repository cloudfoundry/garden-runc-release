package guardiancmd_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/guardian/guardiancmd"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("LoadSeccompProfile", func() {
	var (
		fallback *specs.LinuxSeccomp
		tmpDir   string
	)

	writeProfile := func(content string) string {
		p := filepath.Join(tmpDir, "seccomp.json")
		Expect(os.WriteFile(p, []byte(content), 0600)).To(Succeed())
		return p
	}

	BeforeEach(func() {
		tmpDir = GinkgoT().TempDir()
		fallback = &specs.LinuxSeccomp{
			DefaultAction: specs.ActErrno,
			Syscalls:      []specs.LinuxSyscall{guardiancmd.AllowSyscall("read")},
		}
	})

	Context("when the path is empty", func() {
		It("returns the compiled-in fallback unchanged", func() {
			out, err := guardiancmd.LoadSeccompProfile("", fallback)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).To(BeIdenticalTo(fallback))
		})
	})

	Context("when the file does not exist", func() {
		It("returns an error instead of falling back", func() {
			_, err := guardiancmd.LoadSeccompProfile(filepath.Join(tmpDir, "missing.json"), fallback)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the file is not valid JSON", func() {
		It("returns an error", func() {
			_, err := guardiancmd.LoadSeccompProfile(writeProfile("{not valid json"), fallback)
			Expect(err).To(HaveOccurred())
		})
	})

	Context("when the file contains a valid profile", func() {
		It("replaces the fallback with the loaded profile", func() {
			p := writeProfile(`{
				"defaultAction":"SCMP_ACT_ERRNO",
				"architectures":["SCMP_ARCH_X86_64"],
				"syscalls":[{"names":["read","write"],"action":"SCMP_ACT_ALLOW"}]
			}`)

			out, err := guardiancmd.LoadSeccompProfile(p, fallback)
			Expect(err).NotTo(HaveOccurred())
			Expect(out).NotTo(BeIdenticalTo(fallback))
			Expect(out.DefaultAction).To(Equal(specs.ActErrno))
			Expect(out.Architectures).To(Equal([]specs.Arch{specs.ArchX86_64}))
			Expect(out.Syscalls).To(HaveLen(1))
			Expect(out.Syscalls[0].Names).To(Equal([]string{"read", "write"}))
			Expect(out.Syscalls[0].Action).To(Equal(specs.ActAllow))
		})
	})
})
