package cgroups_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("CPUCgrouper cleanup regression", func() {
	var (
		cpuCgrouper cgroups.CPUCgrouper
		rootPath    string
	)

	BeforeEach(func() {
		var err error
		rootPath, err = os.MkdirTemp("", "cgroup-cleanup-test")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		cpuCgrouper = cgroups.NewCPUCgrouper(rootPath)
	})

	AfterEach(func() {
		os.RemoveAll(rootPath)
	})

	Context("when the cgroup directory contains nested sub-cgroups", func() {
		var badPath, goodPath string

		BeforeEach(func() {
			badPath = filepath.Join(rootPath, cgroups.BadCgroupName, "test-handle")
			goodPath = filepath.Join(rootPath, cgroups.GoodCgroupName, "test-handle")
			Expect(os.MkdirAll(filepath.Join(badPath, "child1", "grandchild"), 0755)).To(Succeed())
			Expect(os.MkdirAll(filepath.Join(badPath, "child2"), 0755)).To(Succeed())
			Expect(os.MkdirAll(goodPath, 0755)).To(Succeed())
		})

		It("removes all cgroups from the bottom up", func() {
			Expect(cpuCgrouper.CleanupCgroups("test-handle")).To(Succeed())
			Expect(badPath).NotTo(BeADirectory())
			Expect(goodPath).NotTo(BeADirectory())
		})
	})

	Context("when bad-cgroup removal fails", func() {
		var badPath, goodPath string

		BeforeEach(func() {
			badPath = filepath.Join(rootPath, cgroups.BadCgroupName, "test-handle")
			goodPath = filepath.Join(rootPath, cgroups.GoodCgroupName, "test-handle")
			Expect(os.MkdirAll(badPath, 0755)).To(Succeed())
			Expect(os.MkdirAll(goodPath, 0755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(badPath, "ordinary-file"), []byte(""), 0644)).To(Succeed())
		})

		It("returns the error without removing the good cgroup", func() {
			Expect(cpuCgrouper.CleanupCgroups("test-handle")).To(MatchError(ContainSubstring("directory not empty")))
			Expect(badPath).To(BeADirectory())
			Expect(goodPath).To(BeADirectory())
		})
	})

	Context("when the cgroup directory does not exist", func() {
		It("succeeds without error", func() {
			Expect(cpuCgrouper.CleanupCgroups("nonexistent-handle")).To(Succeed())
		})
	})

	Context("when only the bad cgroup exists", func() {
		BeforeEach(func() {
			badPath := filepath.Join(rootPath, cgroups.BadCgroupName, "partial-handle")
			Expect(os.MkdirAll(badPath, 0755)).To(Succeed())
		})

		It("removes the bad cgroup and tolerates missing good cgroup", func() {
			Expect(cpuCgrouper.CleanupCgroups("partial-handle")).To(Succeed())
			Expect(filepath.Join(rootPath, cgroups.BadCgroupName, "partial-handle")).NotTo(BeADirectory())
		})
	})
})
