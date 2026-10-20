package bundlerules_test

import (
	spec "code.cloudfoundry.org/guardian/gardener/container-spec"
	"code.cloudfoundry.org/guardian/rundmc/bundlerules"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("CgroupNamespace", func() {
	var (
		initialBndl goci.Bndl
		unified     bool
		rule        bundlerules.CgroupNamespace
	)

	BeforeEach(func() {
		unified = true
		// Start from a bundle that already carries the namespaces the base
		// bundle would set, so we can assert they are preserved.
		initialBndl = goci.Bundle().
			WithNamespace(goci.MountNamespace).
			WithNamespace(goci.UserNamespace)
	})

	JustBeforeEach(func() {
		rule = bundlerules.CgroupNamespace{
			IsCgroup2UnifiedMode: func() bool { return unified },
		}
	})

	Context("when the host runs a cgroup v2 unified hierarchy", func() {
		BeforeEach(func() {
			unified = true
		})

		It("adds the cgroup namespace to an unprivileged container", func() {
			transformedBndl, err := rule.Apply(initialBndl, spec.DesiredContainerSpec{Privileged: false})
			Expect(err).NotTo(HaveOccurred())

			Expect(transformedBndl.Namespaces()).To(ContainElement(
				specs.LinuxNamespace{Type: specs.CgroupNamespace},
			))
		})

		It("preserves the namespaces already set on the bundle", func() {
			transformedBndl, err := rule.Apply(initialBndl, spec.DesiredContainerSpec{Privileged: false})
			Expect(err).NotTo(HaveOccurred())

			Expect(transformedBndl.Namespaces()).To(ContainElements(
				specs.LinuxNamespace{Type: specs.MountNamespace},
				specs.LinuxNamespace{Type: specs.UserNamespace},
			))
		})

		It("adds the cgroup namespace to a privileged container", func() {
			transformedBndl, err := rule.Apply(initialBndl, spec.DesiredContainerSpec{Privileged: true})
			Expect(err).NotTo(HaveOccurred())

			Expect(transformedBndl.Namespaces()).To(ContainElement(
				specs.LinuxNamespace{Type: specs.CgroupNamespace},
			))
		})
	})

	Context("when the host runs cgroup v1 (not unified)", func() {
		BeforeEach(func() {
			unified = false
		})

		It("does not add the cgroup namespace to an unprivileged container", func() {
			transformedBndl, err := rule.Apply(initialBndl, spec.DesiredContainerSpec{Privileged: false})
			Expect(err).NotTo(HaveOccurred())

			Expect(transformedBndl.Namespaces()).NotTo(ContainElement(
				specs.LinuxNamespace{Type: specs.CgroupNamespace},
			))
		})
	})

	Context("when the cgroup v2 detector is not configured", func() {
		JustBeforeEach(func() {
			rule = bundlerules.CgroupNamespace{}
		})

		It("does not add the cgroup namespace", func() {
			transformedBndl, err := rule.Apply(initialBndl, spec.DesiredContainerSpec{Privileged: false})
			Expect(err).NotTo(HaveOccurred())

			Expect(transformedBndl.Namespaces()).NotTo(ContainElement(
				specs.LinuxNamespace{Type: specs.CgroupNamespace},
			))
		})
	})
})
