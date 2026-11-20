//go:build !windows

package nerd

import (
	"context"
	"errors"
	"testing"
	"time"

	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd/nerdfakes"
	"code.cloudfoundry.org/lager/v3/lagertest"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/containerd/errdefs/pkg/errgrpc"
	"google.golang.org/grpc"
)

type createdDeleteService struct {
	tasksapi.TasksClient
	delete func(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error)
}

func (service createdDeleteService) Delete(ctx context.Context, request *tasksapi.DeleteTaskRequest, _ ...grpc.CallOption) (*tasksapi.DeleteResponse, error) {
	return service.delete(ctx, request)
}

func TestDeleteCreatedTask(t *testing.T) {
	for _, testCase := range []struct {
		name       string
		rpcError   error
		closeError error
		wantClosed bool
	}{
		{name: "success", wantClosed: true},
		{name: "already absent", rpcError: errgrpc.ToGRPC(errdefs.ErrNotFound), wantClosed: true},
		{name: "runtime failure", rpcError: errgrpc.ToGRPC(errdefs.ErrFailedPrecondition)},
		{name: "cancelled", rpcError: context.Canceled},
		{name: "close failure", closeError: errors.New("close failed"), wantClosed: true},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			taskIO := &nerdfakes.FakeIO{}
			taskIO.CloseReturns(testCase.closeError)
			called := false
			service := createdDeleteService{delete: func(ctx context.Context, request *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
				called = true
				if request.ContainerID != "created-task" || taskIO.CancelCallCount() != 0 || taskIO.CloseCallCount() != 0 {
					t.Fatal("Delete must precede IO cancellation and target exact task")
				}
				return &tasksapi.DeleteResponse{}, testCase.rpcError
			}}
			mp := metrics.NewMetricsProvider(lagertest.NewTestLogger("created-task"), t.TempDir())
			mp.RegisterUnkillableContainer("created-task")
			err := deleteCreatedTask(context.Background(), service, "created-task", taskIO, mp)
			if (mp.UnkillableContainers() == 0) != testCase.wantClosed {
				t.Fatal("unkillable metric did not track accepted task deletion independently of IO close")
			}
			if !called {
				t.Fatal("public Delete was not called")
			}
			wantError := testCase.closeError != nil || (testCase.rpcError != nil && !testCase.wantClosed)
			if (err != nil) != wantError {
				t.Fatalf("error = %v, want error %v", err, wantError)
			}
			if (taskIO.CancelCallCount() == 1 && taskIO.WaitCallCount() == 1 && taskIO.CloseCallCount() == 1) != testCase.wantClosed {
				t.Fatalf("unexpected IO lifecycle cancel=%d wait=%d close=%d", taskIO.CancelCallCount(), taskIO.WaitCallCount(), taskIO.CloseCallCount())
			}
		})
	}
}

func TestDeleteCreatedTaskLoadedIO(t *testing.T) {
	deleted := false
	closed := false
	taskIO, err := cio.Load(cio.NewFIFOSet(cio.Config{}, func() error {
		if !deleted {
			t.Fatal("loaded IO closed before public task deletion")
		}
		closed = true
		return nil
	}))
	if err != nil {
		t.Fatal(err)
	}
	service := createdDeleteService{delete: func(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
		deleted = true
		return &tasksapi.DeleteResponse{}, nil
	}}
	if err := deleteCreatedTask(t.Context(), service, "created-task", taskIO, metrics.NewMetricsProvider(lagertest.NewTestLogger("loaded-io"), t.TempDir())); err != nil {
		t.Fatal(err)
	}
	if !deleted || !closed {
		t.Fatalf("loaded IO cleanup incomplete deleted=%t closed=%t", deleted, closed)
	}
}

func TestDeleteCreatedTaskTimeoutMetrics(t *testing.T) {
	for _, testCase := range []struct {
		name           string
		expired        bool
		cancelled      bool
		rpcError       error
		wantUnkillable int
	}{
		{name: "expired deadline", expired: true, rpcError: context.DeadlineExceeded, wantUnkillable: 1},
		{name: "cancelled", cancelled: true, rpcError: context.Canceled},
		{name: "remote deadline", rpcError: context.DeadlineExceeded},
		{name: "successful retry"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if testCase.expired {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithDeadline(ctx, time.Now().Add(-time.Second))
				defer deadlineCancel()
			}
			if testCase.cancelled {
				cancel()
			}
			mp := metrics.NewMetricsProvider(lagertest.NewTestLogger("timeout"), t.TempDir())
			if testCase.rpcError == nil {
				mp.RegisterUnkillableContainer("created-task")
			}
			service := createdDeleteService{delete: func(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
				return &tasksapi.DeleteResponse{}, errgrpc.ToGRPC(testCase.rpcError)
			}}
			err := deleteCreatedTask(ctx, service, "created-task", nil, mp)
			if !errors.Is(err, testCase.rpcError) {
				t.Fatalf("error=%v want=%v", err, testCase.rpcError)
			}
			if got := mp.UnkillableContainers(); got != testCase.wantUnkillable {
				t.Fatalf("unkillable=%d want=%d", got, testCase.wantUnkillable)
			}
		})
	}
}
