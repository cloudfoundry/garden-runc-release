//go:build linux

package cgroups_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/guardian/rundmc/cgroups"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("DefaultCgrouper cleanup regression", func() {
	var (
		defaultCgrouper cgroups.DefaultCgrouper
		rootPath        string
	)

	BeforeEach(func() {
		var err error
		rootPath, err = os.MkdirTemp("", "cgroup-cleanup-test")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		defaultCgrouper = cgroups.NewDefaultCgrouper(rootPath)
	})

	AfterEach(func() {
		os.RemoveAll(rootPath)
	})

	Context("when the handle directory contains nested sub-cgroups", func() {
		var handlePath string

		BeforeEach(func() {
			handlePath = filepath.Join(rootPath, "test-handle")
			Expect(os.MkdirAll(filepath.Join(handlePath, "child", "grandchild"), 0755)).To(Succeed())
		})

		It("removes the cgroups from the bottom up", func() {
			Expect(defaultCgrouper.CleanupCgroups("test-handle")).To(Succeed())
			Expect(handlePath).NotTo(BeADirectory())
		})
	})

	Context("when the handle path cannot be removed", func() {
		var handlePath string

		BeforeEach(func() {
			handlePath = filepath.Join(rootPath, "test-handle")
			Expect(os.MkdirAll(handlePath, 0755)).To(Succeed())
			// An ordinary file does not model a cgroup pseudo-file: it remains
			// after rmdir and provides a deterministic error-propagation case.
			Expect(os.WriteFile(filepath.Join(handlePath, "ordinary-file"), []byte(""), 0644)).To(Succeed())
		})

		It("returns the removal error", func() {
			Expect(defaultCgrouper.CleanupCgroups("test-handle")).To(MatchError(ContainSubstring("directory not empty")))
			Expect(handlePath).To(BeADirectory())
		})
	})

	Context("when the handle directory does not exist", func() {
		It("succeeds without error", func() {
			Expect(defaultCgrouper.CleanupCgroups("nonexistent")).To(Succeed())
		})
	})

	Context("stock behavior: empty cgroup directory", func() {
		BeforeEach(func() {
			Expect(os.MkdirAll(filepath.Join(rootPath, "stock-handle"), 0755)).To(Succeed())
		})

		It("removes the empty directory", func() {
			Expect(defaultCgrouper.CleanupCgroups("stock-handle")).To(Succeed())
			Expect(filepath.Join(rootPath, "stock-handle")).NotTo(BeADirectory())
		})
	})
})
