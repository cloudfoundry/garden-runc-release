//go:build !windows

package nerd_test

import (
	"context"
	"errors"
	"net"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd"
	"code.cloudfoundry.org/lager/v3/lagertest"
	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type retryContainersServer struct {
	containersapi.UnimplementedContainersServer
	absent           *atomic.Bool
	missingContainer bool
}

func (rpc *retryContainersServer) Get(_ context.Context, request *containersapi.GetContainerRequest) (*containersapi.GetContainerResponse, error) {
	if rpc.missingContainer && rpc.absent.Load() {
		return nil, status.Error(codes.NotFound, "container absent after timed-out Delete")
	}
	return &containersapi.GetContainerResponse{Container: &containersapi.Container{ID: request.ID}}, nil
}

type retryTasksServer struct {
	tasksapi.UnimplementedTasksServer
	absent   *atomic.Bool
	finished chan struct{}
}

func (rpc *retryTasksServer) Get(_ context.Context, request *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	if rpc.absent.Load() {
		return nil, status.Error(codes.NotFound, "task absent after timed-out Delete")
	}
	return &tasksapi.GetResponse{Process: &tasktypes.Process{ContainerID: request.ContainerID, ID: request.ContainerID, Pid: 123, Status: tasktypes.Status_CREATED}}, nil
}

func (rpc *retryTasksServer) Delete(ctx context.Context, _ *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
	<-ctx.Done()
	rpc.absent.Store(true)
	close(rpc.finished)
	return nil, status.FromContextError(ctx.Err()).Err()
}

func TestDeleteCreatedTaskRetryNotFound(t *testing.T) {
	for _, missingContainer := range []bool{false, true} {
		name := "task absent"
		if missingContainer {
			name = "container absent"
		}
		t.Run(name, func(t *testing.T) {
			socket := filepath.Join(t.TempDir(), "containerd.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			var absent atomic.Bool
			taskRPC := &retryTasksServer{absent: &absent, finished: make(chan struct{})}
			rpcServer := grpc.NewServer()
			containersapi.RegisterContainersServer(rpcServer, &retryContainersServer{absent: &absent, missingContainer: missingContainer})
			tasksapi.RegisterTasksServer(rpcServer, taskRPC)
			serveResult := make(chan error, 1)
			go func() { serveResult <- rpcServer.Serve(listener) }()
			t.Cleanup(func() {
				rpcServer.Stop()
				if err := <-serveResult; err != nil && err != grpc.ErrServerStopped {
					t.Error(err)
				}
			})
			containerdClient, err := client.New(socket)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { containerdClient.Close() })
			log := lagertest.NewTestLogger("retry-not-found")
			mp := metrics.NewMetricsProvider(log, t.TempDir())
			baseCtx := namespaces.WithNamespace(t.Context(), "garden")
			ctx, cancel := context.WithTimeout(baseCtx, 250*time.Millisecond)
			defer cancel()
			manager := nerd.New(containerdClient, ctx, t.TempDir(), mp, runcontainerd.RunscRuntime)
			if err := manager.Delete(log, "created-task"); !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("first Delete error=%v, want local deadline", err)
			}
			if got := mp.UnkillableContainers(); got != 1 {
				t.Fatalf("after timeout unkillable=%d, want1", got)
			}
			select {
			case <-taskRPC.finished:
			case <-time.After(3 * time.Second):
				t.Fatal("server did not finish timed-out Delete")
			}
			retryCtx, retryCancel := context.WithTimeout(baseCtx, 3*time.Second)
			defer retryCancel()
			retry := nerd.New(containerdClient, retryCtx, t.TempDir(), mp, runcontainerd.RunscRuntime)
			if err := retry.Delete(log, "created-task"); err != nil {
				t.Fatal(err)
			}
			if got := mp.UnkillableContainers(); got != 0 {
				t.Fatalf("after idempotent retry unkillable=%d, want0", got)
			}
		})
	}
}
