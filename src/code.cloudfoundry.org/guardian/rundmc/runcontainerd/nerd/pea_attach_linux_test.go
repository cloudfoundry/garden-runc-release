package nerd_test

import (
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd"
	"code.cloudfoundry.org/lager/v3/lagertest"
	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/typeurl/v2"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

type attachTasksServer struct {
	peaInputTasksServer
	process *tasktypes.Process
}

func (rpc *attachTasksServer) Get(context.Context, *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	return &tasksapi.GetResponse{Process: rpc.process}, nil
}

func (rpc *attachTasksServer) Wait(context.Context, *tasksapi.WaitRequest) (*tasksapi.WaitResponse, error) {
	return &tasksapi.WaitResponse{ExitStatus: 42}, nil
}

func (rpc *attachTasksServer) Kill(context.Context, *tasksapi.KillRequest) (*emptypb.Empty, error) {
	return nil, status.Error(codes.FailedPrecondition, "fixture task already stopped; no host signal")
}

type attachBuffer struct {
	sync.Mutex
	data []byte
}

func (b *attachBuffer) Write(p []byte) (int, error) {
	b.Lock()
	defer b.Unlock()
	b.data = append(b.data, p...)
	return len(p), nil
}

func (b *attachBuffer) String() string {
	b.Lock()
	defer b.Unlock()
	return string(b.data)
}

// NewAttach uses the runtime Get response, not cio.WithTerminal or the OCI
// metadata, to choose FIFO streams. A terminal stderr path must never be opened.
func TestNerdPeaFreshAttachUsesRuntimeTerminal(t *testing.T) {
	for _, terminal := range []bool{false, true} {
		t.Run(map[bool]string{false: "nonterminal", true: "terminal"}[terminal], func(t *testing.T) {
			dir := t.TempDir()
			stdout := filepath.Join(dir, "stdout")
			if err := syscall.Mkfifo(stdout, 0600); err != nil {
				t.Fatal(err)
			}
			stderr := filepath.Join(dir, "stderr")
			if terminal {
				stderr = filepath.Join(dir, "missing-directory", "must-not-open")
			} else if err := syscall.Mkfifo(stderr, 0600); err != nil {
				t.Fatal(err)
			}
			// Keep the runtime endpoints open before Attach; otherwise a reader
			// can observe EOF before this fixture writes its first byte.
			endpoints := map[string]*os.File{}
			for _, path := range []string{stdout, stderr} {
				if terminal && path == stderr {
					continue
				}
				f, err := os.OpenFile(path, os.O_RDWR|syscall.O_NONBLOCK, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				endpoints[path] = f
			}
			encoded, err := typeurl.MarshalAny(&specs.Spec{Process: &specs.Process{Terminal: !terminal}})
			if err != nil {
				t.Fatal(err)
			}
			listener, err := net.Listen("unix", filepath.Join(dir, "containerd.sock"))
			if err != nil {
				t.Fatal(err)
			}
			rpc := &attachTasksServer{process: &tasktypes.Process{ContainerID: "child", ID: "child", Pid: ^uint32(0), Status: tasktypes.Status_STOPPED, Terminal: terminal, Stdout: stdout, Stderr: stderr}}
			server := grpc.NewServer()
			defer server.Stop()
			containersapi.RegisterContainersServer(server, &fixtureContainersServer{stored: &containersapi.Container{ID: "child", Spec: typeurl.MarshalProto(encoded)}})
			tasksapi.RegisterTasksServer(server, rpc)
			go func() { _ = server.Serve(listener) }()
			c, err := client.New(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), "pea-attach"), 5*time.Second)
			defer cancel()
			log := lagertest.NewTestLogger("pea-attach")
			n := nerd.New(c, ctx, dir, metrics.NewMetricsProvider(log, dir), runcontainerd.RunscRuntime)
			var out, errout attachBuffer
			if _, err := n.AttachTask(log, "child", garden.ProcessIO{Stdout: &out, Stderr: &errout}); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := n.Delete(log, "child"); err != nil {
					t.Error(err)
				}
			}()
			write := func(path, text string) {
				if _, err := io.WriteString(endpoints[path], text); err != nil {
					t.Fatal(err)
				}
			}
			write(stdout, "stdout")
			if !terminal {
				write(stderr, "stderr")
			}
			wantErr := map[bool]string{false: "stderr", true: ""}[terminal]
			for out.String() != "stdout" || errout.String() != wantErr {
				select {
				case <-ctx.Done():
					t.Fatalf("attach output: stdout=%q stderr=%q", out.String(), errout.String())
				case <-time.After(time.Millisecond):
				}
			}
		})
	}
}
