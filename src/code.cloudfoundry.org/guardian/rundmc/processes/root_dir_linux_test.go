package processes_test

import (
	"os"
	"path/filepath"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/processes"
	fakes "code.cloudfoundry.org/guardian/rundmc/processes/processesfakes"
	"code.cloudfoundry.org/guardian/rundmc/users"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("ProcBuilder", func() {
	Describe("working directory resolution", func() {
		var (
			envDeterminer *fakes.FakeEnvDeterminer
			procBuilder   *processes.ProcBuilder
			bndl          goci.Bndl
			processSpec   garden.ProcessSpec
			user          *users.ExecUser
			preparedProc  *specs.Process
			tmpRootFS     string
			altDir        string
		)

		BeforeEach(func() {
			var err error
			tmpRootFS, err = os.MkdirTemp("", "rootfs")
			Expect(err).NotTo(HaveOccurred())

			altDir, err = os.MkdirTemp("", "altdir")
			Expect(err).NotTo(HaveOccurred())

			envDeterminer = new(fakes.FakeEnvDeterminer)
			envDeterminer.EnvForReturns([]string{"ENV"})

			bndl = goci.Bundle().WithRootFS(tmpRootFS)
			processSpec = garden.ProcessSpec{}
			user = &users.ExecUser{
				Uid: 1,
				Gid: 2,
			}
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tmpRootFS)).To(Succeed())
			Expect(os.RemoveAll(altDir)).To(Succeed())
		})

		JustBeforeEach(func() {
			procBuilder = processes.NewBuilder(envDeterminer, nil)
			preparedProc = procBuilder.BuildProcess(bndl, processSpec, user)
		})

		Context("when spec.Dir is an absolute symlink", func() {
			Context("and the target resolves within the rootfs", func() {
				BeforeEach(func() {
					targetSubdir := "/rootfs-target-" + filepath.Base(tmpRootFS)
					Expect(os.MkdirAll(filepath.Join(tmpRootFS, targetSubdir), 0755)).To(Succeed())
					Expect(os.Symlink(targetSubdir, filepath.Join(tmpRootFS, "symlink-dir"))).To(Succeed())
					processSpec.Dir = "/symlink-dir"
				})

				It("uses spec.Dir as Cwd", func() {
					Expect(preparedProc.Cwd).To(Equal("/symlink-dir"))
				})
			})

			Context("and the target exists only in alt dir", func() {
				BeforeEach(func() {
					altTarget := filepath.Join(altDir, "target")
					Expect(os.MkdirAll(altTarget, 0755)).To(Succeed())
					Expect(os.Symlink(altTarget, filepath.Join(tmpRootFS, "alt-symlink"))).To(Succeed())
					processSpec.Dir = "/alt-symlink"
				})

				It("falls back to '/'", func() {
					Expect(preparedProc.Cwd).To(Equal("/"))
				})
			})
		})

		Context("when spec.Dir is empty and user.Home is an absolute symlink", func() {
			BeforeEach(func() {
				processSpec.Dir = ""
			})

			Context("and the target resolves within the rootfs", func() {
				BeforeEach(func() {
					targetSubdir := "/rootfs-target-" + filepath.Base(tmpRootFS)
					Expect(os.MkdirAll(filepath.Join(tmpRootFS, targetSubdir), 0755)).To(Succeed())
					Expect(os.Symlink(targetSubdir, filepath.Join(tmpRootFS, "symlink-home"))).To(Succeed())
					user.Home = "/symlink-home"
				})

				It("uses home directory as Cwd", func() {
					Expect(preparedProc.Cwd).To(Equal("/symlink-home"))
				})
			})

			Context("and the target exists only in alt dir", func() {
				BeforeEach(func() {
					altTarget := filepath.Join(altDir, "target")
					Expect(os.MkdirAll(altTarget, 0755)).To(Succeed())
					Expect(os.Symlink(altTarget, filepath.Join(tmpRootFS, "alt-home"))).To(Succeed())
					user.Home = "/alt-home"
				})

				It("falls back to '/'", func() {
					Expect(preparedProc.Cwd).To(Equal("/"))
				})
			})
		})
	})
})
