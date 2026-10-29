package runcontainerd_test

import (
	"errors"
	"strings"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/runcontainerdfakes"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("RunContainerPea", func() {

	var (
		fakePeaManager     *runcontainerdfakes.FakePeaManager
		fakeProcessManager *runcontainerdfakes.FakeProcessManager
		fakeVolumizer      *runcontainerdfakes.FakeVolumizer

		processID string

		runContainerPea *runcontainerd.RunContainerPea
	)

	BeforeEach(func() {
		fakePeaManager = new(runcontainerdfakes.FakePeaManager)
		fakeProcessManager = new(runcontainerdfakes.FakeProcessManager)
		fakeVolumizer = new(runcontainerdfakes.FakeVolumizer)
		processID = "pea-id"
		backingProcess := new(runcontainerdfakes.FakeBackingProcess)
		backingProcess.IDReturns(processID)
		fakeProcessManager.GetTaskReturns(backingProcess, nil)

		var err error
		Expect(err).NotTo(HaveOccurred())

		runContainerPea = runcontainerd.NewRunContainerPea(fakePeaManager, fakeProcessManager, fakeVolumizer)
	})

	Describe("RunPea", func() {
		It("creates, loads and starts the child internally in that order", func() {
			var calls []string
			fakePeaManager.CreateCalls(func(_ lager.Logger, id string, _ goci.Bndl, _ garden.ProcessIO) error {
				calls = append(calls, "create:"+id)
				return nil
			})
			fakeProcessManager.GetTaskCalls(func(_ lager.Logger, id string) (runcontainerd.BackingProcess, error) {
				calls = append(calls, "get:"+id)
				return new(runcontainerdfakes.FakeBackingProcess), nil
			})
			fakePeaManager.StartCalls(func(_ lager.Logger, id string) error {
				calls = append(calls, "start:"+id)
				return nil
			})
			_, err := runContainerPea.RunPea(lagertest.NewTestLogger("test"), processID, goci.Bndl{}, "parent", garden.ProcessIO{}, false, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(calls).To(Equal([]string{"create:pea-id", "get:pea-id", "start:pea-id"}))
			Expect(fakePeaManager.DeleteCallCount()).To(BeZero())
		})

		DescribeTable("rolls back its own child after a failed stage", func(stage string) {
			if stage == "get" {
				fakeProcessManager.GetTaskReturns(nil, errors.New("stage-failure"))
			} else {
				fakePeaManager.StartReturns(errors.New("stage-failure"))
			}
			process, err := runContainerPea.RunPea(lagertest.NewTestLogger("test"), processID, goci.Bndl{}, "parent", garden.ProcessIO{}, false, nil, nil)
			Expect(process).To(BeNil())
			Expect(err).To(MatchError("stage-failure"))
			Expect(fakePeaManager.DeleteCallCount()).To(Equal(1))
			_, deletedID := fakePeaManager.DeleteArgsForCall(0)
			Expect(deletedID).To(Equal(processID))
			Expect(fakePeaManager.RemoveBundleCallCount()).To(Equal(1))
			Expect(fakeVolumizer.DestroyCallCount()).To(BeZero(), "PeaCreator owns failure-time image cleanup")
		}, Entry("task lookup", "get"), Entry("task start", "start"))

		It("retains metadata and reports both errors if runtime rollback fails", func() {
			fakePeaManager.StartReturns(errors.New("start-failure"))
			fakePeaManager.DeleteReturns(errors.New("delete-failure"))
			_, err := runContainerPea.RunPea(lagertest.NewTestLogger("test"), processID, goci.Bndl{}, "parent", garden.ProcessIO{}, false, nil, nil)
			Expect(err.Error()).To(ContainSubstring("start-failure"))
			Expect(err.Error()).To(ContainSubstring("delete-failure"))
			var residue interface{ PeaRootfsInUse() bool }
			Expect(errors.As(err, &residue)).To(BeTrue())
			Expect(fakePeaManager.RemoveBundleCallCount()).To(BeZero())
		})

		It("creates a container using process args", func() {
			bundle := goci.Bndl{Spec: specs.Spec{Version: "test-version"}}
			_, err := runContainerPea.RunPea(lagertest.NewTestLogger("test-logger"), processID, bundle, "sandbox-id", garden.ProcessIO{}, false, strings.NewReader(""), nil)
			Expect(err).NotTo(HaveOccurred())

			Expect(fakePeaManager.CreateCallCount()).To(Equal(1))
			_, actualProcessID, actualBundle, _ := fakePeaManager.CreateArgsForCall(0)
			Expect(actualProcessID).To(Equal(processID))
			Expect(actualBundle.Spec.Version).To(Equal("test-version"))
			Expect(actualBundle.Spec.Annotations["container-type"]).To(Equal("pea"))
			Expect(actualBundle.Spec.Annotations["sandbox-container"]).To(Equal("sandbox-id"))
		})

		When("the creator returns an error", func() {
			BeforeEach(func() {
				fakePeaManager.CreateReturns(errors.New("error-creating"))
			})

			It("returns the error", func() {
				_, err := runContainerPea.RunPea(lagertest.NewTestLogger("test-logger"), processID, goci.Bndl{}, "", garden.ProcessIO{}, false, strings.NewReader(""), nil)

				Expect(err).To(MatchError("error-creating"))
				Expect(fakePeaManager.StartCallCount()).To(BeZero())
				Expect(fakePeaManager.DeleteCallCount()).To(BeZero())
			})
		})
	})
})
