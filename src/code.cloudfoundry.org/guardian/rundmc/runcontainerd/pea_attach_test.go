package runcontainerd_test

import (
	"bytes"
	"errors"
	"io"
	"sync"
	"testing"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/runcontainerdfakes"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	"github.com/opencontainers/runtime-spec/specs-go"
)

type missingTaskAttacher struct {
	*runcontainerdfakes.FakeProcessManager
}

type availableTaskAttacher struct {
	*runcontainerdfakes.FakeProcessManager
	attachments int
}

func (m *availableTaskAttacher) AttachTask(_ lager.Logger, id string, _ garden.ProcessIO) (runcontainerd.BackingProcess, error) {
	m.attachments++
	p := &runcontainerdfakes.FakeBackingProcess{}
	p.IDReturns(id)
	return p, nil
}

func (m missingTaskAttacher) AttachTask(_ lager.Logger, id string, _ garden.ProcessIO) (runcontainerd.BackingProcess, error) {
	return nil, runcontainerd.TaskNotFoundError{Handle: id}
}

func TestPeaImageCleanupRetainsOwnershipAndRetriesOnce(t *testing.T) {
	log := lagertest.NewTestLogger("pea-image-retry")
	backing := &runcontainerdfakes.FakeBackingProcess{}
	backing.IDReturns("child")
	backing.WaitReturns(42, nil)
	manager := &runcontainerdfakes.FakePeaManager{}
	volume := &runcontainerdfakes.FakeVolumizer{}
	volume.DestroyReturnsOnCall(0, errors.New("image busy"))
	p := runcontainerd.NewPeaProcess(log, *runcontainerd.NewProcess(log, backing, false), manager, volume)
	if _, err := p.Wait(); err == nil || manager.RemoveBundleCallCount() != 0 {
		t.Fatal("failed image deletion lost ownership metadata")
	}
	if code, err := p.Wait(); err != nil || code != 42 || manager.DeleteCallCount() != 1 || manager.RemoveBundleCallCount() != 1 || volume.DestroyCallCount() != 2 {
		t.Fatalf("image retry = %d, %v", code, err)
	}
}

func TestPeaRestartCleansMissingTaskWithoutInventingWait(t *testing.T) {
	log := lagertest.NewTestLogger("pea-orphan")
	manager := &runcontainerdfakes.FakeContainerManager{}
	manager.SpecReturns(&specs.Spec{Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}, nil)
	processes := missingTaskAttacher{&runcontainerdfakes.FakeProcessManager{}}
	runtime := runcontainerd.New(manager, processes, nil, nil, nil, nil, false, nil, nil, nil, false, nil)
	volume := &runcontainerdfakes.FakeVolumizer{}
	volume.DestroyReturnsOnCall(0, errors.New("image busy"))
	runtime.SetPeaRunner(runcontainerd.NewRunContainerPea(runtime, processes, volume))
	if err := runtime.WaitAndCleanPea(log, "child"); err == nil || manager.RemoveBundleCallCount() != 0 {
		t.Fatal("orphan cleanup lost ownership before deleting image")
	}
	if err := runtime.WaitAndCleanPea(log, "child"); err != nil || manager.RemoveBundleCallCount() != 1 || volume.DestroyCallCount() != 2 || processes.GetTaskCallCount() != 0 {
		t.Fatalf("orphan cleanup = %v", err)
	}
}

func TestPeaConcurrentWaitAndCleanupRetry(t *testing.T) {
	log := lagertest.NewTestLogger("pea-wait")
	backing := &runcontainerdfakes.FakeBackingProcess{}
	backing.IDReturns("child")
	backing.WaitReturns(42, nil)
	manager := &runcontainerdfakes.FakePeaManager{}
	volume := &runcontainerdfakes.FakeVolumizer{}
	p := runcontainerd.NewPeaProcess(log, *runcontainerd.NewProcess(log, backing, false), manager, volume)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if code, err := p.Wait(); err != nil || code != 42 {
				t.Errorf("Wait = %d, %v", code, err)
			}
		}()
	}
	wg.Wait()
	if backing.WaitCallCount() != 1 || manager.DeleteCallCount() != 1 || manager.RemoveBundleCallCount() != 1 || volume.DestroyCallCount() != 1 {
		t.Fatal("concurrent Wait duplicated cleanup")
	}
	if err := p.Signal(garden.SignalKill); err == nil || backing.SignalCallCount() != 0 {
		t.Fatal("finished handle could signal a reused ID")
	}
	// A failed runtime deletion must retain the image and allow a cleanup retry.
	manager = &runcontainerdfakes.FakePeaManager{}
	manager.DeleteReturnsOnCall(0, errors.New("delete failed"))
	volume = &runcontainerdfakes.FakeVolumizer{}
	backing = &runcontainerdfakes.FakeBackingProcess{}
	backing.IDReturns("retry")
	backing.WaitReturns(42, nil)
	p = runcontainerd.NewPeaProcess(log, *runcontainerd.NewProcess(log, backing, false), manager, volume)
	if _, err := p.Wait(); err == nil || volume.DestroyCallCount() != 0 {
		t.Fatal("failed cleanup deleted the image")
	}
	if code, err := p.Wait(); err != nil || code != 42 || backing.WaitCallCount() != 1 || volume.DestroyCallCount() != 1 {
		t.Fatalf("cleanup retry = %d, %v", code, err)
	}
}

func TestPeaAttachUsesChildAndSingleOutputOwner(t *testing.T) {
	log := lagertest.NewTestLogger("pea-attach")
	manager := &runcontainerdfakes.FakeContainerManager{}
	processes := &runcontainerdfakes.FakeProcessManager{}
	backing := &runcontainerdfakes.FakeBackingProcess{}
	backing.IDReturns("child")
	backing.WaitReturns(42, nil)
	processes.GetTaskReturns(backing, nil)
	runtime := runcontainerd.New(manager, processes, nil, nil, nil, nil, false, &runcontainerdfakes.FakeCgroupManager{}, nil, nil, false, nil)
	volume := &runcontainerdfakes.FakeVolumizer{}
	runner := runcontainerd.NewRunContainerPea(runtime, processes, volume)
	runtime.SetPeaRunner(runner)
	var primary, attached bytes.Buffer
	p, err := runner.RunPea(log, "child", goci.Bndl{Spec: specs.Spec{Linux: &specs.Linux{}}}, "parent", garden.ProcessIO{Stdout: &primary}, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	manager.SpecReturns(&specs.Spec{Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}, nil)
	a, err := runtime.Attach(log, "parent", "child", garden.ProcessIO{Stdout: &attached})
	if err != nil || a != p {
		t.Fatalf("Attach = %v, %v", a, err)
	}
	if _, err := runtime.Attach(log, "other-parent", "child", garden.ProcessIO{}); err == nil {
		t.Fatal("cross-parent attachment succeeded")
	}
	if _, err := runtime.Attach(log, "parent", "child", garden.ProcessIO{Stdin: bytes.NewBufferString("input")}); err == nil {
		t.Fatal("unsupported stdin silently accepted")
	}
	input, writer := io.Pipe()
	defer writer.Close()
	if a, err := runtime.Attach(log, "parent", "child", garden.ProcessIO{Stdin: input}); err != nil || a != p {
		t.Fatalf("Garden transport pipe attachment = %v, %v", a, err)
	}
	if n, err := writer.Write([]byte("must not reach startup stdin")); err == nil || n != 0 {
		t.Fatalf("attached stdin was not closed: bytes=%d error=%v", n, err)
	}
	_, _, _, _, _, pio := manager.CreateArgsForCall(0)
	_, out, _ := pio()
	if _, err := out.Write([]byte("child-output")); err != nil {
		t.Fatal(err)
	}
	if primary.String() != "child-output" || attached.String() != "child-output" || processes.GetTaskCallCount() != 1 || processes.GetProcessCallCount() != 0 {
		t.Fatal("Attach stole output or looked up parent exec")
	}
	if err := a.Signal(garden.SignalTerminate); err != nil || backing.SignalCallCount() != 1 {
		t.Fatalf("attached Signal = %v", err)
	}
	if err := runtime.WaitAndCleanPea(log, "child"); err != nil {
		t.Fatal(err)
	}
	if code, err := p.Wait(); err != nil || code != 42 || manager.DeleteCallCount() != 1 || volume.DestroyCallCount() != 1 {
		t.Fatalf("shared restart/Attach Wait = %d, %v", code, err)
	}
}

func TestPeaAttachRejectsMissingParentBeforeOpeningChildIO(t *testing.T) {
	for _, failure := range []error{runcontainerd.ContainerNotFoundError{Handle: "parent"}, errors.New("metadata unavailable")} {
		t.Run(failure.Error(), func(t *testing.T) {
			log := lagertest.NewTestLogger("pea-attach-parent")
			manager := &runcontainerdfakes.FakeContainerManager{}
			manager.SpecCalls(func(_ lager.Logger, id string) (*specs.Spec, error) {
				if id == "parent" {
					return nil, failure
				}
				return &specs.Spec{Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}, nil
			})
			processes := &availableTaskAttacher{FakeProcessManager: &runcontainerdfakes.FakeProcessManager{}}
			runtime := runcontainerd.New(manager, processes, nil, nil, nil, nil, false, nil, nil, nil, false, nil)
			runtime.SetPeaRunner(runcontainerd.NewRunContainerPea(runtime, processes, &runcontainerdfakes.FakeVolumizer{}))
			process, err := runtime.Attach(log, "parent", "child", garden.ProcessIO{})
			if process != nil || err == nil || processes.attachments != 0 || processes.GetTaskCallCount() != 0 || processes.GetProcessCallCount() != 0 {
				t.Fatalf("missing parent opened child IO: process=%v error=%v", process, err)
			}
			var missing garden.ContainerNotFoundError
			if _, notFound := failure.(runcontainerd.ContainerNotFoundError); notFound {
				if !errors.As(err, &missing) || missing.Handle != "parent" {
					t.Fatalf("wrong missing-parent error: %v", err)
				}
			} else if !errors.Is(err, failure) {
				t.Fatalf("parent lookup error was hidden: %v", err)
			}
		})
	}
}
