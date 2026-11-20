//lint:file-ignore SA1019 - we still specify LimitInShares to make the deprecated logic work until we get rid of the code in garden

package peas_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/gardener/gardenerfakes"
	"code.cloudfoundry.org/guardian/rundmc/depot/depotfakes"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/peas"
	"code.cloudfoundry.org/guardian/rundmc/peas/peasfakes"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runrunc/runruncfakes"
	"code.cloudfoundry.org/guardian/rundmc/users"
	"code.cloudfoundry.org/guardian/rundmc/users/usersfakes"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("PeaCreator", func() {
	const imageURI = "some-image-uri"

	var (
		volumizer        *peasfakes.FakeVolumizer
		peaCleaner       *gardenerfakes.FakePeaCleaner
		pidGetter        *peasfakes.FakePidGetter
		networkDepot     *peasfakes.FakeNetworkDepot
		bundleGenerator  *peasfakes.FakeBundleGenerator
		bundleSaver      *depotfakes.FakeBundleSaver
		processBuilder   *runruncfakes.FakeProcessBuilder
		execRunner       *runruncfakes.FakeExecRunner
		privilegedGetter *peasfakes.FakePrivilegedGetter

		peaCreator *peas.PeaCreator

		ctrHandle    string
		ctrBundleDir string
		log          *lagertest.TestLogger

		generatedBundle   = goci.Bndl{Spec: specs.Spec{Version: "our-bundle"}}
		defaultBindMounts = []garden.BindMount{{
			SrcPath: "/some/src",
			DstPath: "/some/dest",
			Mode:    garden.BindMountModeRO,
		}}
		specifiedBindMounts = []garden.BindMount{{
			SrcPath: "/some/src2",
			DstPath: "/some/dest2",
			Mode:    garden.BindMountModeRW,
		}}
		builtProcess *specs.Process
		processSpec  garden.ProcessSpec
		pio          = garden.ProcessIO{Stdin: bytes.NewBufferString("something")}
	)

	BeforeEach(func() {
		volumizer = new(peasfakes.FakeVolumizer)
		peaCleaner = new(gardenerfakes.FakePeaCleaner)
		volumizer.CreateReturns(specs.Spec{
			Version: "some-spec-version",
			Root:    &specs.Root{Path: "/rootfs/path"},
		}, nil)
		pidGetter = new(peasfakes.FakePidGetter)
		pidGetter.GetPidReturns(123, nil)
		networkDepot = new(peasfakes.FakeNetworkDepot)
		networkDepot.SetupBindMountsReturns(defaultBindMounts, nil)

		bundleGenerator = new(peasfakes.FakeBundleGenerator)
		bundleGenerator.GenerateReturns(generatedBundle, nil)
		bundleSaver = new(depotfakes.FakeBundleSaver)

		processBuilder = new(runruncfakes.FakeProcessBuilder)
		builtProcess = &specs.Process{Cwd: "some-cwd"}
		processBuilder.BuildProcessReturns(builtProcess)

		execRunner = new(runruncfakes.FakeExecRunner)

		privilegedGetter = new(peasfakes.FakePrivilegedGetter)
		privilegedGetter.PrivilegedReturns(false, nil)

		peaCreator = &peas.PeaCreator{
			Volumizer:        volumizer,
			PidGetter:        pidGetter,
			NetworkDepot:     networkDepot,
			BundleGenerator:  bundleGenerator,
			BundleSaver:      bundleSaver,
			ProcessBuilder:   processBuilder,
			ExecRunner:       execRunner,
			PrivilegedGetter: privilegedGetter,
			PeaCleaner:       peaCleaner,
		}

		var err error
		ctrHandle = "pea-creator-tests"
		ctrBundleDir, err = os.MkdirTemp("", "pea-creator-tests")
		Expect(err).NotTo(HaveOccurred())
		log = lagertest.NewTestLogger("peas-unit-tests")
		processSpec = garden.ProcessSpec{
			ID:   "some-id",
			Dir:  "/some/dir",
			User: "4:5",
			Image: garden.ImageRef{
				URI:      imageURI,
				Username: "cakeuser",
				Password: "cakepassword",
			},
			BindMounts: specifiedBindMounts,
		}
	})

	AfterEach(func() {
		Expect(os.RemoveAll(ctrBundleDir)).To(Succeed())
	})

	Describe("image-rootfs users and failed-stage cleanup", func() {
		var imageUsers *usersfakes.FakeUserLookupper

		BeforeEach(func() {
			imageUsers = new(usersfakes.FakeUserLookupper)
			imageUsers.LookupReturns(&users.ExecUser{}, nil)
			peaCreator.RootfsUserLookupper = imageUsers
		})

		It("keeps stock uid:gid parsing and ignores the image user lookup", func() {
			processSpec.User = "1:2"
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
			Expect(imageUsers.LookupCallCount()).To(BeZero())
			_, _, user := processBuilder.BuildProcessArgsForCall(0)
			Expect(user.Uid).To(Equal(1))
			Expect(user.Gid).To(Equal(2))
		})

		It("uses image-mapped gofer namespaces and the virtual parent PID namespace for grouped runsc", func() {
			peaCreator.GroupedRunsc = true
			mapping := []specs.LinuxIDMapping{{ContainerID: 0, HostID: 100000, Size: 65536}}
			bundleGenerator.GenerateReturns(goci.Bundle().WithNamespaces(goci.PIDNamespace, goci.UserNamespace, goci.MountNamespace).
				WithUIDMappings(mapping...).WithGIDMappings(mapping...), nil)
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
			Expect(bundleGenerator.GenerateArgsForCall(0).Namespaces).To(Equal(map[string]string{
				"mount": "", "network": "/proc/123/ns/net", "ipc": "", "uts": "", "user": "",
			}))
			_, _, bundle, _, _, _, _, _ := execRunner.RunPeaArgsForCall(0)
			Expect(bundle.Namespaces()).To(Equal([]specs.LinuxNamespace{goci.UserNamespace, goci.MountNamespace}))
			Expect(bundle.UIDMappings()).To(Equal(mapping))
			Expect(bundle.GIDMappings()).To(Equal(mapping))
		})

		It("does not request a user namespace for a privileged grouped runsc pea", func() {
			peaCreator.GroupedRunsc = true
			privilegedGetter.PrivilegedReturns(true, nil)
			bundleGenerator.GenerateReturns(goci.Bundle(), nil)
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
			Expect(bundleGenerator.GenerateArgsForCall(0).Namespaces).To(Equal(map[string]string{
				"mount": "", "network": "/proc/123/ns/net", "ipc": "", "uts": "",
			}))
		})

		It("rejects nonzero limits for a grouped runsc pea before preparing its image", func() {
			peaCreator.GroupedRunsc = true
			bundleGenerator.GenerateReturns(goci.Bundle(), nil)
			for _, limits := range []garden.ProcessLimits{
				{CPU: garden.CPULimits{Weight: 1}},
				{CPU: garden.CPULimits{LimitInShares: 1}},
				{Memory: garden.MemoryLimits{LimitInBytes: 1}},
			} {
				processSpec.OverrideContainerLimits = &limits
				_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
				Expect(err).To(MatchError(ContainSubstring("independent pea limits are unsupported")))
			}
			Expect(volumizer.CreateCallCount()).To(BeZero())
			Expect(execRunner.RunPeaCallCount()).To(BeZero())
		})

		It("accepts empty override limits for a grouped runsc pea", func() {
			peaCreator.GroupedRunsc = true
			bundleGenerator.GenerateReturns(goci.Bundle(), nil)
			processSpec.OverrideContainerLimits = &garden.ProcessLimits{}
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
			Expect(execRunner.RunPeaCallCount()).To(Equal(1))
		})

		It("resolves the child user after preparing its own image", func() {
			peaCreator.GroupedRunsc = true
			bundleGenerator.GenerateReturns(goci.Bundle(), nil)
			lookup := new(usersfakes.FakeUserLookupper)
			lookup.LookupReturns(&users.ExecUser{Uid: 1234, Gid: 4321, Home: "/child-home", Sgids: []int{5678}}, nil)
			peaCreator.RootfsUserLookupper = lookup
			processSpec.User = "childuser"
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
			root, name := lookup.LookupArgsForCall(0)
			Expect(root).To(Equal("/rootfs/path"))
			Expect(name).To(Equal("childuser"))
			_, _, user := processBuilder.BuildProcessArgsForCall(0)
			Expect(user.Uid).To(Equal(1234))
			Expect(user.Gid).To(Equal(4321))
			Expect(user.Sgids).To(Equal([]int{5678}))
		})

		It("destroys the prepared volume if image user lookup fails", func() {
			peaCreator.GroupedRunsc = true
			lookup := new(usersfakes.FakeUserLookupper)
			lookup.LookupReturns(nil, errors.New("missing-image-user"))
			peaCreator.RootfsUserLookupper = lookup
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).To(MatchError(ContainSubstring("missing-image-user")))
			Expect(volumizer.DestroyCallCount()).To(Equal(1))
			Expect(execRunner.RunPeaCallCount()).To(BeZero())
		})

		It("destroys the volume if network bind preparation fails", func() {
			networkDepot.SetupBindMountsReturns(nil, errors.New("bind-failure"))
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).To(MatchError(ContainSubstring("bind-failure")))
			Expect(volumizer.DestroyCallCount()).To(Equal(1))
		})

		It("retains the image when runtime rollback has failed", func() {
			execRunner.RunPeaReturns(nil, &runcontainerd.PeaRuntimeCleanupError{Err: errors.New("runtime-residue")})
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).To(MatchError("runtime-residue"))
			Expect(volumizer.DestroyCallCount()).To(BeZero())
		})
	})

	Describe("pea creation succeeding", func() {
		JustBeforeEach(func() {
			// We don't bother testing that some fake garden.Process is returned by
			// the mock ExecRunner, we leave this verification to our integration
			// tests.
			_, err := peaCreator.CreatePea(log, processSpec, pio, ctrHandle)
			Expect(err).NotTo(HaveOccurred())
		})

		It("checks the sandbox container's privilege", func() {
			Expect(privilegedGetter.PrivilegedCallCount()).To(Equal(1))
			actualId := privilegedGetter.PrivilegedArgsForCall(0)
			Expect(actualId).To(Equal(ctrHandle))
		})

		It("creates a volume", func() {
			Expect(volumizer.CreateCallCount()).To(Equal(1))
			_, actualSpec := volumizer.CreateArgsForCall(0)
			Expect(actualSpec.Handle).To(Equal(processSpec.ID))
			Expect(actualSpec.Image).To(Equal(garden.ImageRef{
				URI:      imageURI,
				Username: "cakeuser",
				Password: "cakepassword",
			}))
			Expect(actualSpec.Privileged).To(Equal(false))
		})

		It("sets up bind mounts", func() {
			Expect(networkDepot.SetupBindMountsCallCount()).To(Equal(1))
			_, actualHandle, actualPrivileged, actualRootfsPath := networkDepot.SetupBindMountsArgsForCall(0)
			Expect(actualHandle).To(Equal(ctrHandle))
			Expect(actualPrivileged).To(BeFalse())
			Expect(actualRootfsPath).To(Equal("/rootfs/path"))
		})

		It("passes bind mounts to bundle generator", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			Expect(actualCtrSpec.BindMounts).To(Equal(append(specifiedBindMounts, defaultBindMounts...)))
		})

		It("passes the processID as handle to the bundle generator", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			Expect(actualCtrSpec.Handle).To(Equal(processSpec.ID))
		})

		It("generates a runtime spec from the VolumeCreator's runtimeSpec", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			Expect(actualCtrSpec.BaseConfig).To(Equal(specs.Spec{
				Version: "some-spec-version",
				Root:    &specs.Root{Path: "/rootfs/path"},
				Windows: &specs.Windows{
					Network: &specs.WindowsNetwork{
						NetworkSharedContainerName: "pea-creator-tests",
					},
				},
			}))
		})

		It("passes <container-handle>/<process-id> as cgroup path to the bundle generator", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			expected := filepath.Join(ctrHandle, processSpec.ID)
			Expect(actualCtrSpec.CgroupPath).To(Equal(expected))
		})

		It("passes sandbox handle to bundle generator", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			Expect(actualCtrSpec.BaseConfig.Windows.Network.NetworkSharedContainerName).To(Equal(ctrHandle))
		})

		It("passes Privileged to bundle generator", func() {
			Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
			actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
			Expect(actualCtrSpec.Privileged).To(Equal(false))
		})

		Describe("sharing namespaces", func() {
			It("shares all namespaces apart from mnt with the container", func() {
				Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
				actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
				Expect(actualCtrSpec.Namespaces).To(Equal(map[string]string{
					"mount":   "",
					"network": "/proc/123/ns/net",
					"user":    "/proc/123/ns/user",
					"ipc":     "/proc/123/ns/ipc",
					"pid":     "/proc/123/ns/pid",
					"uts":     "/proc/123/ns/uts",
				}))
			})

			Context("when sandbox container is privileged", func() {
				BeforeEach(func() {
					privilegedGetter.PrivilegedReturns(true, nil)
				})

				It("shares all namespaces apart from mnt and user with the container", func() {
					Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
					actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
					Expect(actualCtrSpec.Namespaces).To(Equal(map[string]string{
						"mount":   "",
						"network": "/proc/123/ns/net",
						"ipc":     "/proc/123/ns/ipc",
						"pid":     "/proc/123/ns/pid",
						"uts":     "/proc/123/ns/uts",
					}))
				})
			})
		})

		It("builds a process", func() {
			Expect(processBuilder.BuildProcessCallCount()).To(Equal(1))
			actualBundle, actualProcessSpec, user := processBuilder.BuildProcessArgsForCall(0)
			Expect(actualBundle).To(Equal(generatedBundle))
			Expect(actualProcessSpec).To(Equal(processSpec))
			Expect(user.Uid).To(Equal(4))
			Expect(user.Gid).To(Equal(5))
		})

		It("creates a runc container based on the bundle", func() {
			Eventually(execRunner.RunPeaCallCount()).Should(Equal(1))
			_, actualProcessID, actualBundle, actualSandboxHandle, actualPio, actualTTY, actualProcJSON, _ := execRunner.RunPeaArgsForCall(0)
			Expect(actualProcessID).To(Equal(processSpec.ID))
			Expect(actualSandboxHandle).To(Equal(ctrHandle))
			Expect(actualPio).To(Equal(pio))
			Expect(actualTTY).To(BeFalse())
			Expect(actualProcJSON).To(BeNil())
			Expect(actualBundle).To(Equal(generatedBundle.WithProcess(*builtProcess)))
		})

		Context("when the runtime spec uses a TTY", func() {
			BeforeEach(func() {
				builtProcess.Terminal = true
			})

			It("runs with one", func() {
				Eventually(execRunner.RunPeaCallCount()).Should(Equal(1))
				_, _, _, _, _, actualTTY, _, _ := execRunner.RunPeaArgsForCall(0)
				Expect(actualTTY).To(BeTrue())
			})
		})

		It("cleans up the pea", func() {
			Eventually(execRunner.RunPeaCallCount()).Should(Equal(1))
			_, _, _, _, _, _, _, cleanup := execRunner.RunPeaArgsForCall(0)
			Expect(cleanup()).To(Succeed())
			Expect(peaCleaner.CleanCallCount()).To(Equal(1))
			_, processID := peaCleaner.CleanArgsForCall(0)
			Expect(processID).To(Equal(processSpec.ID))
		})

		builtProcess = &specs.Process{Cwd: "some-cwd"}

		Context("when no working dir is specified", func() {
			BeforeEach(func() {
				processSpec.Dir = ""
			})

			It("defaults to /", func() {
				Expect(processBuilder.BuildProcessCallCount()).To(Equal(1))
				_, actualProcessSpec, _ := processBuilder.BuildProcessArgsForCall(0)
				Expect(actualProcessSpec.Dir).To(Equal("/"))
			})
		})

		Context("when no user is specified", func() {
			BeforeEach(func() {
				processSpec.User = ""
			})

			It("defaults to 0:0", func() {
				Expect(processBuilder.BuildProcessCallCount()).To(Equal(1))
				_, _, user := processBuilder.BuildProcessArgsForCall(0)
				Expect(user.Uid).To(Equal(0))
				Expect(user.Gid).To(Equal(0))
			})
		})

		Context("when limits are provided", func() {
			BeforeEach(func() {
				processSpec.OverrideContainerLimits = &garden.ProcessLimits{
					CPU:    garden.CPULimits{LimitInShares: 1},
					Memory: garden.MemoryLimits{LimitInBytes: 2},
				}
			})

			It("provides an explicit cgroup path to bundle generation", func() {
				Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
				actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
				Expect(actualCtrSpec.CgroupPath).To(Equal(processSpec.ID))
			})

			It("sets the memory and CPU limits, and no other limits", func() {
				Expect(bundleGenerator.GenerateCallCount()).To(Equal(1))
				actualCtrSpec := bundleGenerator.GenerateArgsForCall(0)
				Expect(actualCtrSpec.Limits).To(Equal(garden.Limits{
					CPU:    processSpec.OverrideContainerLimits.CPU,
					Memory: processSpec.OverrideContainerLimits.Memory,
				}))
			})
		})
	})

	Describe("pea creation failing", func() {
		var (
			createErr error
		)

		JustBeforeEach(func() {
			_, createErr = peaCreator.CreatePea(log, processSpec, garden.ProcessIO{}, ctrHandle)
		})

		Context("when the network depot returns an error", func() {
			BeforeEach(func() {
				networkDepot.SetupBindMountsReturns(nil, errors.New("explode"))
			})

			It("returns a wrapped error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("explode")))
			})
		})

		Context("when the pid getter returns an error", func() {
			BeforeEach(func() {
				pidGetter.GetPidReturns(-1, errors.New("pickle"))
			})

			It("returns a wrapped error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("pickle")))
			})
		})

		Context("when the bundle generator returns an error", func() {
			BeforeEach(func() {
				bundleGenerator.GenerateReturns(goci.Bndl{}, errors.New("banana"))
			})

			It("returns a wrapped error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("banana")))
			})

			It("invokes cleanup on the volumizer", func() {
				Expect(volumizer.DestroyCallCount()).To(Equal(1))
			})

			Context("and volumizer.Destroy returns an error", func() {
				BeforeEach(func() {
					volumizer.DestroyReturns(errors.New("Pikachu!"))
				})

				It("contains both error strings in the returned error", func() {
					Expect(createErr.Error()).To(ContainSubstring("Pikachu!"))
					Expect(createErr.Error()).To(ContainSubstring("banana"))
				})
			})
		})

		Context("when the volume creator returns an error", func() {
			BeforeEach(func() {
				volumizer.CreateReturns(specs.Spec{}, errors.New("coconut"))
			})

			It("returns a wrapped error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("coconut")))
			})
		})

		Context("when the user is specified as a username, not a uid:gid", func() {
			BeforeEach(func() {
				processSpec.User = "frank"
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("frank")))
			})
		})

		Context("when the exec runner returns an error", func() {
			BeforeEach(func() {
				execRunner.RunPeaReturns(nil, errors.New("execrunner-error"))
			})

			It("returns an error", func() {
				Expect(createErr.Error()).To(ContainSubstring("execrunner-error"))
			})

			It("does not leak the pea process dir", func() {
				Expect(filepath.Join(ctrBundleDir, "processes", "some-id")).ToNot(BeADirectory())
			})

			It("invokes cleanup on the volumizer", func() {
				Expect(volumizer.DestroyCallCount()).To(Equal(1))
			})

			Context("and volumizer.Destroy returns an error", func() {
				BeforeEach(func() {
					volumizer.DestroyReturns(errors.New("Pikachu!"))
				})

				It("contains both error strings in the returned error", func() {
					Expect(createErr.Error()).To(ContainSubstring("Pikachu!"))
					Expect(createErr.Error()).To(ContainSubstring("execrunner-error"))
				})
			})
		})

		Context("when the privileged getter returns an error", func() {
			BeforeEach(func() {
				privilegedGetter.PrivilegedReturns(false, errors.New("privileged-getter-error"))
			})

			It("returns an error", func() {
				Expect(createErr).To(MatchError(ContainSubstring("privileged-getter-error")))
			})
		})
	})
})
