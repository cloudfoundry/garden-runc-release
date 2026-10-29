//go:build !windows

package nerd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/lager/v3"
	apievents "github.com/containerd/containerd/api/events"
	v2types "github.com/containerd/containerd/api/types/runc/options"
	"github.com/containerd/containerd/v2/client"
	ctrdevents "github.com/containerd/containerd/v2/core/events"
	"github.com/containerd/containerd/v2/pkg/cio"
	"github.com/containerd/errdefs"
	"github.com/containerd/typeurl/v2"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/anypb"

	specs "github.com/opencontainers/runtime-spec/specs-go"
)

type Nerd struct {
	client    *client.Client
	context   context.Context
	ioFifoDir string
	mp        *metrics.MetricsProvider
	policy    runcontainerd.RuntimePolicy
	peaIOMu   sync.Mutex
	peaTasks  map[string]*peaTask
}

type peaTask struct {
	task      client.Task
	stdinDone <-chan struct{}
	terminal  bool
	context   context.Context
	cancel    context.CancelFunc
}

func New(containerdClient *client.Client, context context.Context, ioFifoDir string, mp *metrics.MetricsProvider, policy runcontainerd.RuntimePolicy) *Nerd {
	return &Nerd{
		client:    containerdClient,
		context:   context,
		ioFifoDir: ioFifoDir,
		mp:        mp,
		policy:    policy,
		peaTasks:  make(map[string]*peaTask),
	}
}

func (n *Nerd) Create(log lager.Logger, containerID string, spec *specs.Spec, hostUID, hostGID uint32, pio func() (io.Reader, io.Writer, io.Writer)) (result error) {
	log.Debug("creating-container", lager.Data{"containerID": containerID})
	if n.policy == runcontainerd.RunscRuntime {
		spec.Windows = nil // runsc rejects the Windows section
		if spec.Annotations == nil {
			spec.Annotations = make(map[string]string)
		}
		if spec.Annotations["container-type"] == "pea" {
			parent := spec.Annotations["sandbox-container"]
			if parent == "" || parent == containerID {
				return fmt.Errorf("pea %s requires a distinct sandbox-container", containerID)
			}
			// Garden's labels also drive discovery and cleanup. Translate them at
			// the runtime boundary so image-backed peas join the parent's Sentry.
			spec.Annotations["io.kubernetes.cri.container-type"] = "container"
			spec.Annotations["io.kubernetes.cri.sandbox-id"] = parent
		} else if _, exists := spec.Annotations["io.kubernetes.cri.container-type"]; !exists {
			spec.Annotations["io.kubernetes.cri.container-type"] = "sandbox"
		}
	}
	containerOpts := []client.NewContainerOpts{client.WithSpec(spec)}
	if n.policy == runcontainerd.RunscRuntime {
		containerOpts = append(containerOpts, client.WithRuntime(runcontainerd.SupportedRuntimeType, runscRuntimeOptions()))
	}
	container, err := n.client.NewContainer(n.context, containerID, containerOpts...)
	if err != nil {
		return err
	}
	// Only a successful NewContainer establishes ownership. Failed label/task
	// setup must not leave owned metadata behind, or erase a colliding ID.
	defer func() {
		if result == nil {
			return
		}
		if err := n.Delete(log, containerID); err != nil {
			result = &runcontainerd.PeaRuntimeCleanupError{Err: errors.Join(result, err)}
			return
		}
		result = errors.Join(result, n.RemoveBundle(log, containerID))
	}()

	for key, value := range spec.Annotations {
		log.Debug("setting-label", lager.Data{"containerID": containerID, "label-key": key, "label-value": value})
		if _, err := container.SetLabels(n.context, map[string]string{key: value}); err != nil {
			return err
		}
	}

	log.Debug("creating-task", lager.Data{"containerID": containerID})
	var taskOpts []client.NewTaskOpts
	if n.policy == runcontainerd.StockRuntime {
		// Stock runc needs explicit keyring and UID/GID task options;
		// runsc manages its own keyring and UID/GID via the OCI spec.
		taskOpts = append(taskOpts, client.WithNoNewKeyring, WithUIDAndGID(hostUID, hostGID))
	}
	isPea := spec.Annotations["container-type"] == "pea"
	processIO := noTTYProcessIO(pio)
	var inputDone chan struct{}
	if isPea {
		inputDone = make(chan struct{})
		processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
			stdin, stdout, stderr := pio()
			return &peaInput{reader: orEmpty(stdin), done: inputDone}, stdout, stderr, spec.Process != nil && spec.Process.Terminal
		}
	}
	task, err := container.NewTask(n.context, cio.NewCreator(withProcessIO(processIO, n.ioFifoDir)), taskOpts...)
	if err == nil && isPea {
		ctx, cancel := context.WithCancel(n.context)
		n.peaIOMu.Lock()
		n.peaTasks[containerID] = &peaTask{task: task, stdinDone: inputDone, terminal: spec.Process != nil && spec.Process.Terminal, context: ctx, cancel: cancel}
		n.peaIOMu.Unlock()
	}
	return err
}

func runscRuntimeOptions() *anypb.Any {
	configPath := protowire.AppendTag(nil, 2, protowire.BytesType)
	configPath = protowire.AppendString(configPath, "/etc/containerd/runsc/config.toml")
	return &anypb.Any{TypeUrl: "runtimeoptions.v1.Options", Value: configPath}
}

// Start starts a task created by Create. Keeping the two phases separate lets
// Garden configure networking after the task's namespaces exist but before its
// init process runs, which runtimes that snapshot the network at start need.
func (n *Nerd) Start(log lager.Logger, containerID string) error {
	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return err
	}

	n.peaIOMu.Lock()
	pea := n.peaTasks[containerID]
	n.peaIOMu.Unlock()
	log.Debug("starting-task", lager.Data{"containerID": containerID})
	if err := task.Start(n.context); err != nil {
		return err
	}

	if pea != nil {
		// A terminal has one bidirectional console master, not an independent
		// stdin pipe. Closing its keepalive on input EOF makes the stock shim
		// close that master, dropping output and delivering SIGHUP. Keep it
		// until task exit/deletion; stdin completion is not terminal shutdown.
		if pea.terminal {
			return nil
		}
		// The shim holds a keepalive writer on stdin. Release it only after
		// cio has written the supplied input, never immediately after Start.
		go func() {
			select {
			case <-pea.stdinDone:
				if err := task.CloseIO(pea.context, client.WithStdinCloser); err != nil {
					log.Error("failed-closing-pea-stdin", err, lager.Data{"containerID": containerID})
				}
			case <-pea.context.Done():
			}
		}()
		return nil
	}
	return task.CloseIO(n.context, client.WithStdinCloser)
}

func withProcessKillLogging(log lager.Logger, mp *metrics.MetricsProvider) func(context.Context, client.Process) error {
	return func(ctx context.Context, p client.Process) error {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()
		// ignore errors to wait and kill as we are forcefully killing
		// the process and don't care about the exit status
		log.Debug("with-process-kill.wait", lager.Data{"containerdProcess": p.ID(), "processID": p.Pid()})
		s, err := p.Wait(ctx)
		if err != nil {
			return err
		}
		log.Debug("with-process-kill.kill", lager.Data{"containerdProcess": p.ID(), "processID": p.Pid()})
		if err := p.Kill(ctx, syscall.SIGKILL); err != nil {
			if errdefs.IsFailedPrecondition(err) || errdefs.IsNotFound(err) {
				return nil
			}
			return err
		}
		log.Debug("with-process-kill.kill-complete", lager.Data{"containerdProcess": p.ID(), "processID": p.Pid()})
		// wait for the process to fully stop before letting the rest of the deletion complete
		select {
		case <-s:
			mp.RegisterKillableContainer(p.ID())
			break
		case <-time.After(time.Minute * 2):
			mp.RegisterUnkillableContainer(p.ID())
			return fmt.Errorf("Container still exists 2 minutes after kill -9. This container is unkillable and a VM reboot is required: containerdProcess=%s, processID=%d", p.ID(), p.Pid())
		}

		log.Debug("with-process-kill.wait-complete", lager.Data{"containerdProcess": p.ID(), "processID": p.Pid()})
		return nil
	}
}

func (n *Nerd) Delete(log lager.Logger, containerID string) (result error) {
	defer func() {
		if result == nil {
			result = n.releasePeaIO(containerID)
		}
	}()
	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		switch err.(type) {
		case runcontainerd.ContainerNotFoundError:
			log.Debug("container-already-deleted", lager.Data{"containerID": containerID})
			n.mp.RegisterKillableContainer(containerID)
			return nil
		case runcontainerd.TaskNotFoundError:
			log.Debug("task-already-deleted", lager.Data{"containerID": containerID})
			n.mp.RegisterKillableContainer(containerID)
			return nil
		}
		return err
	}

	log.Debug("deleting-task", lager.Data{"containerID": containerID})
	status, err := task.Status(n.context)
	if err != nil {
		return err
	}
	if status.Status == client.Created && task.Pid() != 0 && n.policy == runcontainerd.RunscRuntime {
		ctx, cancel := context.WithTimeout(n.context, 2*time.Minute)
		defer cancel()
		if err := deleteCreatedTask(ctx, n.client.TaskService(), containerID, task.IO(), n.mp); err != nil {
			return err
		}
		return nil
	}
	_, err = task.Delete(n.context, withProcessKillLogging(log, n.mp))
	return err
}

func (n *Nerd) State(log lager.Logger, containerID string) (int, string, error) {
	log = log.Session("state", lager.Data{"containerID": containerID})

	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return 0, "", err
	}

	log.Debug("getting-task-status")
	status, err := n.tryToGetStatus(log, task)
	if err != nil {
		return 0, "", err
	}

	log.Debug("task-result", lager.Data{"pid": strconv.Itoa(int(task.Pid())), "status": string(status.Status)})
	return int(task.Pid()), string(status.Status), nil
}

func (n *Nerd) tryToGetTask(log lager.Logger, container client.Container) (client.Task, error) {
	log = log.Session("try-to-get-task")

	const retries = 5
	for i := 0; i < retries; i++ {
		task, err := container.Task(n.context, cio.Load)
		if err != nil {
			if errdefs.IsNotFound(err) {
				return nil, runcontainerd.TaskNotFoundError{Handle: container.ID()}
			}
			if errors.Is(err, context.DeadlineExceeded) {
				log.Info("retrying-after-task-error", lager.Data{"retry-number": i + 1, "error": err.Error()})
				time.Sleep(500 * time.Millisecond)
				continue
			}
			return nil, err
		}

		if task.Pid() != 0 {
			return task, nil
		}

		log.Info("retrying-after-pid-0-response", lager.Data{"retry-number": i + 1})
		time.Sleep(500 * time.Millisecond)
	}

	return nil, errors.New("failed getting task")
}

func (n *Nerd) tryToGetStatus(log lager.Logger, task client.Task) (client.Status, error) {
	log = log.Session("try-to-get-status", lager.Data{"taskID": task.ID()})

	const retries = 5
	for i := 0; i < retries; i++ {
		status, err := task.Status(n.context)
		if err != nil {
			return client.Status{}, err
		}

		if status.Status != client.Unknown {
			return status, nil
		}

		log.Info("retrying-after-status-unknown-response", lager.Data{"retry-number": i + 1})
		time.Sleep(500 * time.Millisecond)
	}

	return client.Status{}, errors.New("failed getting task status")
}

func (n *Nerd) Exec(log lager.Logger, containerID, processID string, spec *specs.Process, processIO func() (io.Reader, io.Writer, io.Writer, bool)) (runcontainerd.BackingProcess, error) {
	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return BackingProcess{}, err
	}

	log.Debug("execing-task", lager.Data{"containerID": containerID, "processID": processID})
	process, err := task.Exec(n.context, processID, spec, cio.NewCreator(withProcessIO(processIO, n.ioFifoDir)))
	if err != nil {
		return BackingProcess{}, err
	}

	log.Debug("starting-task", lager.Data{"containerID": containerID, "processID": processID})
	if err := process.Start(n.context); err != nil {
		if _, deleteErr := process.Delete(n.context); deleteErr != nil {
			log.Error("delete-failed-exec", deleteErr, lager.Data{"containerID": containerID, "processID": processID})
			return BackingProcess{}, errors.Join(err, fmt.Errorf("delete failed exec %s: %w", processID, deleteErr))
		}
		return BackingProcess{}, err
	}

	log.Debug("closing-stdin", lager.Data{"containerID": containerID, "processID": processID})
	go exponentialBackoffCloseIO(process, n.context, log, containerID)

	return NewBackingProcess(log, process, n.context), nil
}

func (n *Nerd) GetProcess(log lager.Logger, containerID, processID string) (runcontainerd.BackingProcess, error) {
	log.Debug("get-process", lager.Data{"containerID": containerID, "processID": processID})
	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return BackingProcess{}, err
	}

	process, err := task.LoadProcess(n.context, processID, cio.Load)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return BackingProcess{}, runcontainerd.ProcessNotFoundError{Handle: containerID, ID: processID}
		}
		return BackingProcess{}, err
	}

	return NewBackingProcess(log, process, n.context), nil
}

func (n *Nerd) GetTask(log lager.Logger, id string) (runcontainerd.BackingProcess, error) {
	log.Debug("get-task", lager.Data{"id": id})
	n.peaIOMu.Lock()
	created := n.peaTasks[id]
	n.peaIOMu.Unlock()
	if created != nil {
		// Wait must drain the actual creator's IO, not cio.Load's no-op Wait.
		return NewBackingProcess(log, created.task, n.context), nil
	}
	_, task, err := n.loadContainerAndTask(log, id)
	if err != nil {
		return BackingProcess{}, err
	}

	return NewBackingProcess(log, task, n.context), nil
}

func (n *Nerd) AttachTask(log lager.Logger, id string, pio garden.ProcessIO) (runcontainerd.BackingProcess, error) {
	container, err := n.loadContainer(log, id)
	if err != nil {
		return nil, err
	}
	n.peaIOMu.Lock()
	defer n.peaIOMu.Unlock()
	if n.peaTasks[id] != nil {
		return nil, fmt.Errorf("pea %s already has a task IO reader", id)
	}
	task, err := container.Task(n.context, cio.NewAttach(cio.WithStreams(nil, orDiscard(pio.Stdout), orDiscard(pio.Stderr))))
	if err != nil {
		if errdefs.IsNotFound(err) {
			return nil, runcontainerd.TaskNotFoundError{Handle: id}
		}
		return nil, err
	}
	ctx, cancel := context.WithCancel(n.context)
	n.peaTasks[id] = &peaTask{task: task, context: ctx, cancel: cancel}
	return NewBackingProcess(log, task, n.context), nil
}

func (n *Nerd) releasePeaIO(id string) error {
	n.peaIOMu.Lock()
	task := n.peaTasks[id]
	delete(n.peaTasks, id)
	n.peaIOMu.Unlock()
	if task != nil {
		task.cancel()
		if task.task.IO() != nil {
			task.task.IO().Cancel()
			return task.task.IO().Close()
		}
	}
	return nil
}

func exponentialBackoffCloseIO(process client.Process, ctx context.Context, log lager.Logger, containerID string) {
	duration := 3 * time.Second
	retries := 10
	for i := 0; i < retries; i++ {
		if err := process.CloseIO(ctx, client.WithStdinCloser); err != nil {
			log.Error("failed-closing-stdin", err, lager.Data{"containerID": containerID, "processID": process.ID()})
			time.Sleep(duration)
			duration *= 2
			continue
		}
		break
	}
}

func noTTYProcessIO(initProcessIO func() (io.Reader, io.Writer, io.Writer)) func() (io.Reader, io.Writer, io.Writer, bool) {
	return func() (io.Reader, io.Writer, io.Writer, bool) {
		stdin, stdout, stderr := initProcessIO()
		return stdin, stdout, stderr, false
	}
}

func withProcessIO(processIO func() (io.Reader, io.Writer, io.Writer, bool), ioFifoDir string) cio.Opt {
	return func(opt *cio.Streams) {
		stdin, stdout, stderr, hasTTY := processIO()

		cio.WithFIFODir(ioFifoDir)(opt)
		if hasTTY {
			cio.WithTerminal(opt)
			cio.WithStreams(orEmpty(stdin), orDiscard(stdout), nil)(opt)
		} else {
			cio.WithStreams(orEmpty(stdin), orDiscard(stdout), orDiscard(stderr))(opt)
		}
	}
}

func orEmpty(r io.Reader) io.Reader {
	if r != nil {
		return r
	}
	return bytes.NewBuffer(nil)
}

func orDiscard(w io.Writer) io.Writer {
	if w != nil {
		return w
	}
	return io.Discard
}

func (n *Nerd) GetContainerPID(log lager.Logger, containerID string) (uint32, error) {
	_, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return 0, err
	}

	return task.Pid(), nil
}

func (n *Nerd) loadContainer(log lager.Logger, containerID string) (client.Container, error) {
	log.Debug("loading-container", lager.Data{"containerID": containerID})
	container, err := n.client.LoadContainer(n.context, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			log.Debug("container-not-found", lager.Data{"containerID": containerID})
			return nil, runcontainerd.ContainerNotFoundError{Handle: containerID}
		}
		log.Debug("loading-container-failed", lager.Data{"containerID": containerID})
		return nil, err
	}
	return container, nil
}

func (n *Nerd) loadContainers(labels ...runcontainerd.ContainerFilter) ([]client.Container, error) {
	var flattenedLabels []string
	for _, label := range labels {
		flattenedLabels = append(flattenedLabels, fmt.Sprintf("labels.\"%s\"%s%s", label.Label, label.ComparisonOp, label.Value))
	}

	return n.client.Containers(n.context, strings.Join(flattenedLabels, ","))
}

func (n *Nerd) loadContainerAndTask(log lager.Logger, containerID string) (client.Container, client.Task, error) {
	container, err := n.loadContainer(log, containerID)
	if err != nil {
		return nil, nil, err
	}

	log.Debug("loading-task", lager.Data{"containerID": containerID})
	task, err := n.tryToGetTask(log, container)
	if err != nil {
		log.Debug("loading-task-failed", lager.Data{"containerID": containerID})
		return nil, nil, err
	}

	return container, task, nil
}

func (n *Nerd) OOMEvents(log lager.Logger) <-chan *apievents.TaskOOM {
	events, errs := n.client.Subscribe(n.context, `topic=="/tasks/oom"`)
	oomEvents := make(chan *apievents.TaskOOM)

	go func() {
		for {
			select {
			case err, ok := <-errs:
				if !ok {
					log.Info("event service has been closed")
				}

				if err != nil {
					log.Error("event service received an error", err)
				}

				close(oomEvents)
				return

			case e := <-events:
				event, err := coerceEvent(e)
				if err != nil {
					log.Error("failed to coerce containerd event", err, lager.Data{"event": e})
					continue
				}

				log.Debug("received an OOM event", lager.Data{"containerID": event.ContainerID})
				oomEvents <- event
			}
		}
	}()

	return oomEvents
}

func (n *Nerd) Spec(log lager.Logger, containerID string) (*specs.Spec, error) {
	// The OCI spec belongs to metadata, not a live task. Restart cleanup must
	// still discover ownership after task deletion succeeds.
	container, err := n.loadContainer(log, containerID)
	if err != nil {
		return nil, err
	}

	log.Debug("getting-container-spec", lager.Data{"containerID": containerID})
	return container.Spec(n.context)
}

func coerceEvent(event *ctrdevents.Envelope) (*apievents.TaskOOM, error) {
	if event.Event == nil {
		return nil, errors.New("empty event")
	}

	unmarshalledEvent, err := typeurl.UnmarshalAny(event.Event)
	if err != nil {
		return nil, err
	}

	oom, ok := unmarshalledEvent.(*apievents.TaskOOM)
	if !ok {
		return nil, errors.New("unexpected event")
	}

	return oom, nil
}

func (n *Nerd) BundleIDs(labels ...runcontainerd.ContainerFilter) ([]string, error) {
	containers, err := n.loadContainers(labels...)
	if err != nil {
		return nil, err
	}

	handles := []string{}
	for _, container := range containers {
		handles = append(handles, container.ID())
	}

	return handles, nil
}

func (n *Nerd) RemoveBundle(log lager.Logger, handle string) error {
	container, err := n.loadContainer(log, handle)
	if err == nil {
		log.Debug("deleting-container", lager.Data{"containerID": handle})
		return container.Delete(n.context)
	}

	if _, isNotFound := err.(runcontainerd.ContainerNotFoundError); isNotFound {
		log.Debug("container-already-deleted", lager.Data{"containerID": handle})
		return nil
	}

	log.Debug("loading-container-failed", lager.Data{"handle": handle})
	return err
}

func WithUIDAndGID(uid, gid uint32) client.NewTaskOpts {
	return func(ctx context.Context, c *client.Client, ti *client.TaskInfo) error {
		return updateTaskInfoCreateOptions(ti, func(opts *v2types.Options) error {
			opts.IoUid = uid
			opts.IoGid = gid
			return nil
		})
	}
}

func updateTaskInfoCreateOptions(taskInfo *client.TaskInfo, updateCreateOptions func(createOptions *v2types.Options) error) error {
	if taskInfo.Options == nil {
		taskInfo.Options = &v2types.Options{}
	}
	opts, ok := taskInfo.Options.(*v2types.Options)

	if !ok {
		return errors.New("could not cast TaskInfo Options to CreateOptions")
	}

	return updateCreateOptions(opts)
}

// TaskMetrics retrieves raw cgroup metrics for a container via the containerd
// task API. Returns the raw protobuf Any (may be cgroupv1 or cgroupv2 format
// depending on the shim) and the container creation time.
func (n *Nerd) TaskMetrics(log lager.Logger, containerID string) (typeurl.Any, time.Time, error) {
	log = log.Session("task-metrics", lager.Data{"containerID": containerID})

	container, task, err := n.loadContainerAndTask(log, containerID)
	if err != nil {
		return nil, time.Time{}, err
	}

	metric, err := task.Metrics(n.context)
	if err != nil {
		return nil, time.Time{}, fmt.Errorf("task.Metrics: %w", err)
	}

	if metric.Data == nil {
		return nil, time.Time{}, fmt.Errorf("no metrics data for container %s", containerID)
	}

	// Get creation time from container info
	info, err := container.Info(n.context)
	if err != nil {
		// Non-fatal: use zero time (Age will be large but metrics still report)
		log.Error("get-info-for-creation-time", err)
		return metric.Data, time.Time{}, nil
	}

	return metric.Data, info.CreatedAt, nil
}
