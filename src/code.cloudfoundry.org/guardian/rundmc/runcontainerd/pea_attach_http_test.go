package runcontainerd_test

import (
	"bytes"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	gardenclient "code.cloudfoundry.org/garden/client"
	"code.cloudfoundry.org/garden/client/connection"
	"code.cloudfoundry.org/garden/gardenfakes"
	"code.cloudfoundry.org/garden/server"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/runcontainerdfakes"
	"code.cloudfoundry.org/lager/v3/lagertest"
	specs "github.com/opencontainers/runtime-spec/specs-go"
)

func TestPeaOutputOnlyAttachThroughGardenHTTP(t *testing.T) {
	log := lagertest.NewTestLogger("pea-http-attach")
	manager := &runcontainerdfakes.FakeContainerManager{}
	manager.SpecReturns(&specs.Spec{Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}, nil)
	processes := &runcontainerdfakes.FakeProcessManager{}
	backing := &runcontainerdfakes.FakeBackingProcess{}
	backing.IDReturns("child")
	exited := make(chan struct{})
	var exitOnce sync.Once
	finish := func() { exitOnce.Do(func() { close(exited) }) }
	backing.WaitCalls(func() (int, error) { <-exited; return 42, nil })
	backing.SignalCalls(func(signal syscall.Signal) error {
		if signal != syscall.SIGTERM {
			t.Errorf("unexpected signal %v", signal)
		}
		finish()
		return nil
	})
	processes.GetTaskReturns(backing, nil)
	runtime := runcontainerd.New(manager, processes, nil, nil, nil, nil, false, &runcontainerdfakes.FakeCgroupManager{}, nil, nil, false, nil)
	volume := &runcontainerdfakes.FakeVolumizer{}
	runner := runcontainerd.NewRunContainerPea(runtime, processes, volume)
	runtime.SetPeaRunner(runner)
	original, err := runner.RunPea(log, "child", goci.Bndl{Spec: specs.Spec{Linux: &specs.Linux{}}}, "parent", garden.ProcessIO{}, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	container := &gardenfakes.FakeContainer{}
	container.HandleReturns("parent")
	container.AttachCalls(func(id string, pio garden.ProcessIO) (garden.Process, error) {
		return runtime.Attach(log, "parent", id, pio)
	})
	backend := &gardenfakes.FakeBackend{}
	backend.LookupReturns(container, nil)
	backend.ContainersReturns([]garden.Container{container}, nil)
	socket := filepath.Join(t.TempDir(), "garden.sock")
	s := server.New("unix", socket, 0, time.Second, backend, log)
	if err := s.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { finish(); s.Stop() }()
	client := gardenclient.New(connection.New("unix", socket))
	remote, err := client.Lookup("parent")
	if err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	attached, err := remote.Attach("child", garden.ProcessIO{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		t.Fatalf("output-only HTTP attachment rejected server input pipe: %v", err)
	}
	_, _, _, _, _, taskIO := manager.CreateArgsForCall(0)
	_, out, errout := taskIO()
	out.Write([]byte("HTTP_ATTACHED_OUTPUT"))
	errout.Write([]byte("HTTP_ATTACHED_STDERR"))
	if err := attached.Signal(garden.SignalTerminate); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		if code, err := attached.Wait(); err != nil || code != 42 {
			t.Errorf("attached HTTP Wait = %d, %v", code, err)
		}
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("attached HTTP Wait did not finish")
	}
	if stdout.String() != "HTTP_ATTACHED_OUTPUT" || stderr.String() != "HTTP_ATTACHED_STDERR" {
		t.Fatalf("HTTP output = %q, %q", stdout.String(), stderr.String())
	}
	if code, err := original.Wait(); err != nil || code != 42 || backing.WaitCallCount() != 1 || backing.SignalCallCount() != 1 || manager.DeleteCallCount() != 1 || volume.DestroyCallCount() != 1 {
		t.Fatalf("HTTP attachment duplicated Wait/cleanup: %d, %v", code, err)
	}
}
