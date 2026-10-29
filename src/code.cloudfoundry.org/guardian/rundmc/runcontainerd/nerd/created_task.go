//go:build !windows

package nerd

import (
	"context"
	"errors"

	"code.cloudfoundry.org/guardian/metrics"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/containerd/errdefs/pkg/errgrpc"
)

func deleteCreatedTask(ctx context.Context, service tasksapi.TasksClient, containerID string, taskIO cio.IO, mp *metrics.MetricsProvider) error {
	_, err := service.Delete(ctx, &tasksapi.DeleteTaskRequest{ContainerID: containerID})
	if err != nil && !errdefs.IsNotFound(errgrpc.ToNative(err)) {
		if errors.Is(errgrpc.ToNative(err), context.DeadlineExceeded) && ctx.Err() == context.DeadlineExceeded {
			mp.RegisterUnkillableContainer(containerID)
		}
		return errgrpc.ToNative(err)
	}
	mp.RegisterKillableContainer(containerID)
	if taskIO != nil {
		taskIO.Cancel()
		taskIO.Wait()
		if err := taskIO.Close(); err != nil {
			return err
		}
	}
	return nil
}
