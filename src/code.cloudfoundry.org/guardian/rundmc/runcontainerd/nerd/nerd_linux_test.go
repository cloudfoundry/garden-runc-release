package nerd_test

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	apievents "github.com/containerd/containerd/api/events"
	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	runcoptions "github.com/containerd/containerd/api/types/runc/options"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/client"
	"github.com/containerd/containerd/v2/core/containers"
	"github.com/containerd/containerd/v2/core/leases"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/pkg/oci"
	"github.com/containerd/typeurl/v2"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/gbytes"
	"github.com/onsi/gomega/gexec"
	cgrouputils "github.com/opencontainers/cgroups"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/types/known/emptypb"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/metrics"
	"code.cloudfoundry.org/guardian/rundmc"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/nerd"
	"code.cloudfoundry.org/guardian/rundmc/rundmcfakes"
	"code.cloudfoundry.org/guardian/rundmc/users"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
)

type fixtureContainersServer struct {
	containersapi.UnimplementedContainersServer
	created chan *containersapi.Container
	stored  *containersapi.Container
}

func (rpc *fixtureContainersServer) Create(_ context.Context, request *containersapi.CreateContainerRequest) (*containersapi.CreateContainerResponse, error) {
	rpc.stored = request.Container
	rpc.created <- request.Container
	return &containersapi.CreateContainerResponse{Container: request.Container}, nil
}

func (rpc *fixtureContainersServer) Get(context.Context, *containersapi.GetContainerRequest) (*containersapi.GetContainerResponse, error) {
	return &containersapi.GetContainerResponse{Container: rpc.stored}, nil
}

func (rpc *fixtureContainersServer) Update(_ context.Context, request *containersapi.UpdateContainerRequest) (*containersapi.UpdateContainerResponse, error) {
	rpc.stored = request.Container
	return &containersapi.UpdateContainerResponse{Container: request.Container}, nil
}

type policyTasksServer struct {
	tasksapi.UnimplementedTasksServer
	created chan *tasksapi.CreateTaskRequest
	killed  chan string
	deleted chan string
}

func (rpc *policyTasksServer) Delete(_ context.Context, request *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
	rpc.deleted <- request.ContainerID
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before public task deletion")
}

func (rpc *policyTasksServer) Create(_ context.Context, request *tasksapi.CreateTaskRequest) (*tasksapi.CreateTaskResponse, error) {
	rpc.created <- request
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before runtime creation")
}

func (rpc *policyTasksServer) Get(_ context.Context, request *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	return &tasksapi.GetResponse{Process: &tasktypes.Process{
		ContainerID: request.ContainerID, ID: request.ContainerID, Pid: ^uint32(0), Status: tasktypes.Status_CREATED,
	}}, nil
}

func (rpc *policyTasksServer) Wait(_ context.Context, request *tasksapi.WaitRequest) (*tasksapi.WaitResponse, error) {
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before task signaling")
}

func (rpc *policyTasksServer) Kill(_ context.Context, request *tasksapi.KillRequest) (*emptypb.Empty, error) {
	rpc.killed <- request.ContainerID
	return nil, status.Error(codes.PermissionDenied, "policy fixture stops before task signaling")
}

// peaInputTasksServer is a TaskService fixture that never runs a host process.
type peaInputTasksServer struct {
	tasksapi.UnimplementedTasksServer
	created chan *tasksapi.CreateTaskRequest
	closed  chan *tasksapi.CloseIORequest
	killed  atomic.Bool
}

func (rpc *peaInputTasksServer) Create(_ context.Context, request *tasksapi.CreateTaskRequest) (*tasksapi.CreateTaskResponse, error) {
	rpc.created <- request
	return &tasksapi.CreateTaskResponse{ContainerID: request.ContainerID, Pid: ^uint32(0)}, nil
}

func (rpc *peaInputTasksServer) Start(context.Context, *tasksapi.StartRequest) (*tasksapi.StartResponse, error) {
	return &tasksapi.StartResponse{Pid: ^uint32(0)}, nil
}

func (rpc *peaInputTasksServer) CloseIO(_ context.Context, request *tasksapi.CloseIORequest) (*emptypb.Empty, error) {
	rpc.closed <- request
	return &emptypb.Empty{}, nil
}

func (rpc *peaInputTasksServer) Delete(context.Context, *tasksapi.DeleteTaskRequest) (*tasksapi.DeleteResponse, error) {
	return &tasksapi.DeleteResponse{}, nil
}

func (rpc *peaInputTasksServer) Wait(context.Context, *tasksapi.WaitRequest) (*tasksapi.WaitResponse, error) {
	return &tasksapi.WaitResponse{}, nil
}

func (rpc *peaInputTasksServer) Kill(context.Context, *tasksapi.KillRequest) (*emptypb.Empty, error) {
	// Cleanup signals only this fake RPC, never the synthetic host PID.
	rpc.killed.Store(true)
	return &emptypb.Empty{}, nil
}

func (rpc *peaInputTasksServer) Get(_ context.Context, request *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	processStatus := tasktypes.Status_CREATED
	if rpc.killed.Load() {
		processStatus = tasktypes.Status_STOPPED
	}
	return &tasksapi.GetResponse{Process: &tasktypes.Process{
		ContainerID: request.ContainerID, ID: request.ContainerID, Pid: ^uint32(0), Status: processStatus,
	}}, nil
}

func TestNerdPeaCloseIOFollowsInputCompletion(t *testing.T) {
	testNerdPeaInputCompletion(t, false, runcontainerd.RunscRuntime)
}

func TestNerdTerminalPeaInputCompletionKeepsConsole(t *testing.T) {
	for _, policy := range []runcontainerd.RuntimePolicy{runcontainerd.StockRuntime, runcontainerd.RunscRuntime} {
		t.Run(policy.String(), func(t *testing.T) {
			testNerdPeaInputCompletion(t, true, policy)
		})
	}
}

func testNerdPeaInputCompletion(t *testing.T, terminal bool, policy runcontainerd.RuntimePolicy) {
	t.Helper()
	dir := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(dir, "containerd.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	defer server.Stop()
	rpc := &peaInputTasksServer{created: make(chan *tasksapi.CreateTaskRequest, 1), closed: make(chan *tasksapi.CloseIORequest, 2)}
	containersapi.RegisterContainersServer(server, &fixtureContainersServer{created: make(chan *containersapi.Container, 1)})
	tasksapi.RegisterTasksServer(server, rpc)
	go func() { _ = server.Serve(listener) }()
	c, err := client.New(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), "pea-input"), 5*time.Second)
	defer cancel()
	ctx = leases.WithLease(ctx, "fixture-no-lease")
	log := lagertest.NewTestLogger("pea-input")
	n := nerd.New(c, ctx, dir, metrics.NewMetricsProvider(log, dir), policy)
	input, writer := io.Pipe()
	defer input.Close()
	defer writer.Close()
	spec := &specs.Spec{Process: &specs.Process{Args: []string{"/unused"}, Terminal: terminal}, Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}
	if err := n.Create(log, "child", spec, 0, 0, func() (io.Reader, io.Writer, io.Writer) { return input, nil, nil }); err != nil {
		t.Fatal(err)
	}
	defer func() {
		// Also unblock the supplied reader on a failed assertion before releasing
		// the cached cio workers. This fixture never signals a host process.
		_ = input.Close()
		if err := n.Delete(log, "child"); err != nil {
			t.Error(err)
		}
	}()
	request := <-rpc.created
	// A real FIFO endpoint permits cio's final write without requiring a shim.
	fifo, err := os.OpenFile(request.Stdin, os.O_RDWR|syscall.O_NONBLOCK, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer fifo.Close()
	if err := n.Start(log, "child"); err != nil {
		t.Fatal(err)
	}
	select {
	case <-rpc.closed:
		t.Fatal("CloseIO preceded supplied stdin completion")
	case <-time.After(50 * time.Millisecond):
	}
	if _, err := io.WriteString(writer, "last bytes"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if terminal {
		b := make([]byte, 10)
		if _, err := io.ReadFull(fifo, b); err != nil || string(b) != "last bytes" {
			t.Fatalf("final terminal FIFO bytes = %q, %v", b, err)
		}
		select {
		case <-rpc.closed:
			t.Fatal("terminal stdin completion closed the bidirectional console")
		case <-time.After(50 * time.Millisecond):
		}
		return
	}
	select {
	case closed := <-rpc.closed:
		if !closed.Stdin || closed.ContainerID != "child" {
			t.Fatalf("CloseIO request = %+v", closed)
		}
	case <-ctx.Done():
		t.Fatal("CloseIO did not follow supplied stdin completion")
	}
	b := make([]byte, 10)
	if _, err := io.ReadFull(fifo, b); err != nil || string(b) != "last bytes" {
		t.Fatalf("final FIFO bytes = %q, %v", b, err)
	}
}

func TestNerdSpecWithoutTask(t *testing.T) {
	dir := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(dir, "containerd.sock"))
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	defer server.Stop()
	want := &specs.Spec{Annotations: map[string]string{"container-type": "pea", "sandbox-container": "parent"}}
	encoded, err := typeurl.MarshalAny(want)
	if err != nil {
		t.Fatal(err)
	}
	containersapi.RegisterContainersServer(server, &fixtureContainersServer{stored: &containersapi.Container{ID: "orphan", Spec: typeurl.MarshalProto(encoded)}})
	// No TaskService is registered: metadata lookup must not request a task.
	go func() { _ = server.Serve(listener) }()
	c, err := client.New(listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx := namespaces.WithNamespace(context.Background(), "spec-orphan")
	n := nerd.New(c, ctx, dir, metrics.NewMetricsProvider(lagertest.NewTestLogger("metrics"), dir), runcontainerd.RunscRuntime)
	got, err := n.Spec(lagertest.NewTestLogger("spec"), "orphan")
	if err != nil || got == nil || got.Annotations["sandbox-container"] != "parent" {
		t.Fatalf("task-free spec = %+v, %v", got, err)
	}
}

func TestNerdRuntimePolicyCreatePayload(t *testing.T) {
	for _, test := range []struct {
		runtimeType       string
		wantRuntime       string
		wantWindows       bool
		wantOptions       bool
		annotations       map[string]string
		wantContainerType string
		wantSandboxID     string
		wantTTY           bool
	}{
		{wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true},
		{runtimeType: "io.containerd.runc.v2", wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", wantContainerType: "sandbox"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"container-type": "garden-init"}, wantContainerType: "sandbox"},
		{runtimeType: "io.containerd.runc.v2", wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true, annotations: map[string]string{"container-type": "garden-init"}},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"sentinel": "retained"}, wantContainerType: "sandbox"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"io.kubernetes.cri.container-type": "container"}, wantContainerType: "container"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"container-type": "pea", "sandbox-container": "garden-parent", "sentinel": "retained"}, wantContainerType: "container", wantSandboxID: "garden-parent"},
		{runtimeType: "io.containerd.runsc.v1", wantRuntime: "io.containerd.runsc.v1", annotations: map[string]string{"container-type": "pea", "sandbox-container": "garden-parent"}, wantContainerType: "container", wantSandboxID: "garden-parent", wantTTY: true},
		{runtimeType: "io.containerd.runc.v2", wantRuntime: "io.containerd.runc.v2", wantWindows: true, wantOptions: true, annotations: map[string]string{"container-type": "pea", "sandbox-container": "garden-parent"}, wantTTY: true},
	} {
		t.Run(test.runtimeType, func(t *testing.T) {
			socketDir, err := os.MkdirTemp("", "nerd-policy-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(socketDir) })
			socket := filepath.Join(socketDir, "containerd.sock")
			listener, err := net.Listen("unix", socket)
			if err != nil {
				t.Fatal(err)
			}
			containerRPC := &fixtureContainersServer{created: make(chan *containersapi.Container, 1)}
			taskRPC := &policyTasksServer{created: make(chan *tasksapi.CreateTaskRequest, 1), killed: make(chan string, 1), deleted: make(chan string, 1)}
			rpcServer := grpc.NewServer()
			containersapi.RegisterContainersServer(rpcServer, containerRPC)
			tasksapi.RegisterTasksServer(rpcServer, taskRPC)
			serveResult := make(chan error, 1)
			go func() { serveResult <- rpcServer.Serve(listener) }()
			t.Cleanup(func() {
				rpcServer.Stop()
				if err := <-serveResult; err != nil && err != grpc.ErrServerStopped {
					t.Error(err)
				}
			})
			containerdClient, err := client.New(socket, client.WithDefaultRuntime(test.wantRuntime))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { containerdClient.Close() })
			policy, err := runcontainerd.ResolveRuntimePolicy(test.runtimeType)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(namespaces.WithNamespace(context.Background(), "garden"), 5*time.Second)
			defer cancel()
			ctx = leases.WithLease(ctx, "lease-is-off")
			policyLog := lager.NewLogger("nerd-policy")
			manager := nerd.New(containerdClient, ctx, t.TempDir(), metrics.NewMetricsProvider(policyLog, t.TempDir()), policy)
			wantSentinel := test.annotations["sentinel"]
			spec := &specs.Spec{Version: specs.Version, Windows: &specs.Windows{}, Process: &specs.Process{Args: []string{"/policy-sentinel"}, Cwd: "/", Terminal: test.wantTTY}, Annotations: test.annotations}
			err = manager.Create(lager.NewLogger("nerd-policy"), "policy-container", spec, 4242, 4343, func() (io.Reader, io.Writer, io.Writer) { return nil, nil, nil })
			if err == nil || !strings.Contains(err.Error(), "policy fixture stops before runtime creation") {
				t.Fatalf("Create error = %v, want bounded fixture rejection", err)
			}
			// The fixture deliberately exposes a task after failed Create. Owned
			// rollback must reach its runtime-specific delete path before returning.
			rollback := taskRPC.deleted
			if test.wantOptions {
				rollback = taskRPC.killed
			}
			select {
			case id := <-rollback:
				if id != "policy-container" {
					t.Errorf("rollback ID = %q", id)
				}
			default:
				t.Fatal("failed Create did not attempt owned task rollback")
			}
			select {
			case request := <-containerRPC.created:
				if request.ID != "policy-container" || request.Runtime.Name != test.wantRuntime {
					t.Errorf("container identity/runtime = %+v", request)
				}
				if !test.wantOptions {
					options := request.Runtime.Options
					if options == nil || options.TypeUrl != "runtimeoptions.v1.Options" {
						t.Fatalf("runsc runtime options = %v, want runtimeoptions.v1.Options", options)
					}
					field, wireType, tagSize := protowire.ConsumeTag(options.Value)
					if field != 2 || wireType != protowire.BytesType || tagSize < 0 {
						t.Fatalf("runsc config_path tag = %d/%d/%d, want field 2 string", field, wireType, tagSize)
					}
					configPath, pathSize := protowire.ConsumeString(options.Value[tagSize:])
					if configPath != "/etc/containerd/runsc/config.toml" || pathSize < 0 || tagSize+pathSize != len(options.Value) {
						t.Fatalf("runsc config_path payload = %q (%d bytes), want exact configured path only", configPath, pathSize)
					}
				}
				var emitted specs.Spec
				if err := json.Unmarshal(request.Spec.Value, &emitted); err != nil {
					t.Fatal(err)
				}
				if (emitted.Windows != nil) != test.wantWindows || (spec.Windows != nil) != test.wantWindows {
					t.Errorf("Windows section: emitted=%v input=%v, want presence %v", emitted.Windows, spec.Windows, test.wantWindows)
				}
				if len(emitted.Process.Args) != 1 || emitted.Process.Args[0] != "/policy-sentinel" {
					t.Errorf("unrelated process arguments changed: %v", emitted.Process.Args)
				}
				if emitted.Annotations["io.kubernetes.cri.container-type"] != test.wantContainerType {
					t.Errorf("container type = %q, want %q", emitted.Annotations["io.kubernetes.cri.container-type"], test.wantContainerType)
				}
				if emitted.Annotations["io.kubernetes.cri.sandbox-id"] != test.wantSandboxID {
					t.Errorf("sandbox ID = %q, want %q", emitted.Annotations["io.kubernetes.cri.sandbox-id"], test.wantSandboxID)
				}
				if emitted.Annotations["sentinel"] != wantSentinel {
					t.Error("unrelated annotation lost")
				}
			default:
				t.Fatal("no container creation payload")
			}
			select {
			case request := <-taskRPC.created:
				if request.Terminal != test.wantTTY || (test.wantTTY && request.Stderr != "") {
					t.Errorf("task terminal = %t, stderr = %q; want terminal %t", request.Terminal, request.Stderr, test.wantTTY)
				}
				if request.ContainerID != "policy-container" {
					t.Errorf("task container ID = %q", request.ContainerID)
				}
				if test.wantOptions {
					if request.Options == nil {
						t.Fatal("stock task options missing")
					}
					decoded, err := typeurl.UnmarshalAny(request.Options)
					if err != nil {
						t.Fatal(err)
					}
					options, ok := decoded.(*runcoptions.Options)
					if !ok || !options.NoNewKeyring || options.IoUid != 4242 || options.IoGid != 4343 {
						t.Errorf("stock options = %+v, want NoNewKeyring and UID/GID 4242/4343", decoded)
					}
				} else if request.Options != nil {
					t.Errorf("runsc inherited task options: %v", request.Options)
				}
			default:
				t.Fatal("no task creation payload")
			}
			err = manager.Delete(lager.NewLogger("nerd-policy-delete"), "policy-container")
			if test.wantOptions {
				if err == nil || !strings.Contains(err.Error(), "policy fixture stops before task signaling") {
					t.Errorf("stock Delete error = %v, want ordinary task cleanup", err)
				}
				select {
				case id := <-taskRPC.killed:
					if id != "policy-container" {
						t.Errorf("Kill container = %q", id)
					}
				case <-ctx.Done():
					t.Error("stock Delete did not reach ordinary task cleanup before the test deadline")
				}
			} else {
				if err == nil || !strings.Contains(err.Error(), "policy fixture stops before public task deletion") {
					t.Errorf("runsc Delete error = %v, want public TaskService.Delete rejection", err)
				}
				select {
				case id := <-taskRPC.deleted:
					if id != "policy-container" {
						t.Errorf("Delete container = %q", id)
					}
				case <-ctx.Done():
					t.Error("runsc Delete did not reach public task deletion before the test deadline")
				}
				select {
				case id := <-taskRPC.killed:
					t.Errorf("runsc CREATED-task path unexpectedly reached stock Kill for %q", id)
				default:
				}
			}
		})
	}
}

var randomGenerator *rand.Rand
var _ = Describe("Nerd", func() {
	var (
		testLogger    lager.Logger
		cnerd         *nerd.Nerd
		containerID   string
		processID     string
		fifoDir       string
		initProcessIO func() (io.Reader, io.Writer, io.Writer)
		processIO     func() (io.Reader, io.Writer, io.Writer, bool)
		stdin         io.Reader
		stdout        io.Writer
		stderr        io.Writer
		spec          *specs.Spec
		mp            *metrics.MetricsProvider
		rootfsMounted bool
	)

	const (
		maximusUID = 5000
		maximusGID = 5000
	)

	BeforeEach(func() {
		rootfsMounted = false
		randomGenerator = rand.New(rand.NewSource(time.Now().UnixNano()))
		containerID = fmt.Sprintf("test-container-%s", randomString(10))
		processID = fmt.Sprintf("test-process-%s", randomString(10))
		stdout = gbytes.NewBuffer()
		stderr = gbytes.NewBuffer()
		stdin = nil
		initProcessIO = func() (io.Reader, io.Writer, io.Writer) {
			return stdin, stdout, stderr
		}
		processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
			return stdin, stdout, stderr, false
		}
		testLogger = lagertest.NewTestLogger("nerd-test")

		mp = metrics.NewMetricsProvider(testLogger, fifoDir)

		var err error
		fifoDir, err = os.MkdirTemp("", "nerd-fifo")
		Expect(err).NotTo(HaveOccurred())
	})

	JustBeforeEach(func() {
		cnerd = nerd.New(containerdClient, containerdContext, fifoDir, mp, testRuntimePolicy)
	})

	AfterEach(func() {
		Expect(os.RemoveAll(fifoDir)).To(Succeed())
		if spec != nil && spec.Root != nil {
			if rootfsMounted {
				Expect(syscall.Unmount(spec.Root.Path, syscall.MNT_DETACH)).To(Succeed())
			}
			Expect(os.RemoveAll(filepath.Dir(spec.Root.Path))).To(Succeed())
		}
	})

	Describe("exec archive prerequisites and round trip", func() {
		AfterEach(func() {
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
		})

		It("uses rootfs mkdir and tar, preserves content and cleans up execs", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			runtime := new(rundmcfakes.FakeOCIRuntime)
			sequence := 0
			var execIDs []string
			runtime.ExecStub = func(log lager.Logger, handle string, processSpec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				sequence++
				id := fmt.Sprintf("archive-%d", sequence)
				execIDs = append(execIDs, id)
				resolvedUser, err := users.LookupUser(spec.Root.Path, processSpec.User)
				if err != nil {
					return nil, err
				}
				backingProcess, err := cnerd.Exec(log, handle, id, &specs.Process{
					Args: append([]string{processSpec.Path}, processSpec.Args...),
					Cwd:  "/",
					User: specs.User{UID: uint32(resolvedUser.Uid), GID: uint32(resolvedUser.Gid)},
				}, func() (io.Reader, io.Writer, io.Writer, bool) {
					return processIO.Stdin, processIO.Stdout, processIO.Stderr, false
				})
				if err != nil {
					return nil, err
				}
				return runcontainerd.NewProcess(log, backingProcess, true), nil
			}
			streamer := rundmc.NewExecStreamer(runtime)
			payload := bytes.Repeat([]byte("runtime checksum\x00"), 8192)
			var archive bytes.Buffer
			writer := tar.NewWriter(&archive)
			for _, name := range []string{"file with spaces", "--checkpoint=1", "nested/child"} {
				Expect(writer.WriteHeader(&tar.Header{Name: name, Mode: 0644, Size: int64(len(payload))})).To(Succeed())
				_, err := writer.Write(payload)
				Expect(err).NotTo(HaveOccurred())
			}
			Expect(writer.Close()).To(Succeed())
			Expect(streamer.StreamIn(testLogger, containerID, garden.StreamInSpec{Path: "/archive dir", TarStream: bytes.NewReader(archive.Bytes())})).To(Succeed())
			reader, err := streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: "/archive dir/"})
			Expect(err).NotTo(HaveOccurred())
			data, err := io.ReadAll(reader)
			Expect(reader.Close()).To(Succeed())
			Expect(err).NotTo(HaveOccurred())
			archiveReader := tar.NewReader(bytes.NewReader(data))
			files := 0
			for {
				header, err := archiveReader.Next()
				if err == io.EOF {
					break
				}
				Expect(err).NotTo(HaveOccurred())
				if header.Typeflag != tar.TypeReg {
					continue
				}
				contents, err := io.ReadAll(archiveReader)
				Expect(err).NotTo(HaveOccurred())
				Expect(contents).To(Equal(payload))
				files++
			}
			Expect(files).To(Equal(3))
			for _, userName := range []string{"root", "65534", "nobody"} {
				resolvedUser, err := users.LookupUser(spec.Root.Path, userName)
				Expect(err).NotTo(HaveOccurred())
				path := "/tmp/archive-" + userName
				Expect(streamer.StreamIn(testLogger, containerID, garden.StreamInSpec{Path: path, User: userName, TarStream: bytes.NewReader(archive.Bytes())})).To(Succeed())
				reader, err := streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: path + "/", User: userName})
				Expect(err).NotTo(HaveOccurred())
				data, err := io.ReadAll(reader)
				Expect(reader.Close()).To(Succeed())
				Expect(err).NotTo(HaveOccurred())
				archiveReader := tar.NewReader(bytes.NewReader(data))
				files := 0
				for {
					header, err := archiveReader.Next()
					if err == io.EOF {
						break
					}
					Expect(err).NotTo(HaveOccurred())
					if header.Typeflag != tar.TypeReg {
						continue
					}
					Expect(header.Uid).To(Equal(resolvedUser.Uid))
					contents, err := io.ReadAll(archiveReader)
					Expect(err).NotTo(HaveOccurred())
					Expect(contents).To(Equal(payload))
					files++
				}
				Expect(files).To(Equal(3))
			}
			reader, err = streamer.StreamOut(testLogger, containerID, garden.StreamOutSpec{Path: "/archive dir/"})
			Expect(err).NotTo(HaveOccurred())
			Expect(reader.Close()).To(Succeed())
			for _, id := range execIDs {
				Eventually(func() error {
					_, err := cnerd.GetProcess(testLogger, containerID, id)
					return err
				}, "10s").Should(MatchError(runcontainerd.ProcessNotFoundError{Handle: containerID, ID: id}))
			}
			_, state, err := cnerd.State(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(state).To(Equal(string(client.Running)))
		})
	})

	Describe("Create", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("creates the containerd container by id", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			containers := listContainers(testConfig.CtrBin, testConfig.Socket)
			Expect(containers).To(ContainSubstring(containerID))
		})

		It("starts an init process in the container", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processes := listProcesses(testConfig.CtrBin, testConfig.Socket, containerID)
			// init processes look like this in ctr:
			// PID    INFO
			// 12345  -
			Expect(processes).To(MatchRegexp(`\d+\s+-`))
		})

		It("adds the annotations as labels", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Annotations = map[string]string{"hello": "world", "goodbye": "potato"}

			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(containerLabels(testConfig.CtrBin, testConfig.Socket, containerID)).To(HaveKeyWithValue("hello", "world"))
			Expect(containerLabels(testConfig.CtrBin, testConfig.Socket, containerID)).To(HaveKeyWithValue("goodbye", "potato"))
		})

		It("writes stdout", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "echo hi && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stdout, "30s").Should(gbytes.Say("hi"))
		})

		It("writes stderr", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "echo hi 1>&2 && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stderr, "30s").Should(gbytes.Say("hi"))
		})

		It("reads from stdin", func() {
			stdin = bytes.NewBufferString("hi")
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/bin/sh", "-c", "cat && sleep 60"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			Eventually(stdout, "30s").Should(gbytes.Say("hi"))
		})

		It("removes the container metadata when the init process cannot be created", func() {
			if testRuntimePolicy.IsAlternative() {
				Skip("runsc resolves the init executable at start, not at create")
			}
			spec = generateSpec(containerdContext, containerdClient, containerID)
			spec.Process.Args = []string{"/command-does-not-exist"}
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).NotTo(Succeed())

			containers := listContainers(testConfig.CtrBin, testConfig.Socket)
			Expect(containers).NotTo(ContainSubstring(containerID))
		})
	})

	Describe("Spec", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("returns the container spec", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			actualSpec, err := cnerd.Spec(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(actualSpec).To(Equal(spec))
		})

		It("fails when container id not found", func() {
			_, err := cnerd.Spec(testLogger, "notAContainerId")
			Expect(err).To(MatchError("container notAContainerId not found"))
		})

		It("returns the container spec after its task has been deleted", func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())

			actualSpec, err := cnerd.Spec(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(actualSpec).To(Equal(spec))
			Expect(cnerd.RemoveBundle(testLogger, containerID)).To(Succeed())
		})
	})

	Describe("Exec", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("execs a process in the container", func() {
			processSpec := &specs.Process{
				Args: []string{"/bin/sleep", "30"},
				Cwd:  "/",
			}

			proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
			Expect(proc.ID()).To(Equal(processID))

			loaded, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			Expect(loaded.ID()).To(Equal(processID))
		})

		It("removes an exec process that fails to start", func() {
			processSpec := &specs.Process{
				Args: []string{"/command-does-not-exist"},
				Cwd:  "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).To(HaveOccurred())

			_, err = cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).To(MatchError(runcontainerd.ProcessNotFoundError{Handle: containerID, ID: processID}))
		})

		Describe("process IO", func() {
			It("reads from stdin", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/cat", "/proc/self/fd/0"},
					Cwd:  "/",
				}

				stdout := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stdin := gbytes.BufferWithBytes([]byte("hello nerd"))
					stdout := io.MultiWriter(stdout, GinkgoWriter)

					return stdin, stdout, nil, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stdout).Should(gbytes.Say("hello nerd"))
			})

			It("writes to stdout", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/echo", "hello nerd"},
					Cwd:  "/",
				}

				stdout := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stdout := io.MultiWriter(stdout, GinkgoWriter)
					return nil, stdout, nil, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stdout).Should(gbytes.Say("hello nerd"))
			})

			It("writes to stderr", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/cat", "notafile.txt"},
					Cwd:  "/",
				}

				stderr := gbytes.NewBuffer()
				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					stderr := io.MultiWriter(stderr, GinkgoWriter)
					return nil, nil, stderr, false
				}

				_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())
				Eventually(stderr).Should(gbytes.Say("No such file"))
			})

			It("has access to a pseudo terminal", func() {
				processSpec := &specs.Process{
					Args: []string{"/bin/test", "-t", "1"},
					Cwd:  "/",
				}

				processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
					return nil, nil, nil, true
				}

				proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
				Expect(err).NotTo(HaveOccurred())

				exitCode, err := proc.Wait()
				Expect(err).NotTo(HaveOccurred())
				Expect(exitCode).To(BeZero())
			})
		})
	})

	Describe("Wait", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processSpec := &specs.Process{
				Args: []string{"/bin/sh", "-c", "exit 17"},
				Cwd:  "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("succeeds", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})

		It("returns the exit code", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			exitCode, err := proc.Wait()
			Expect(err).NotTo(HaveOccurred())
			Expect(exitCode).To(Equal(17))
		})

		It("allows you to call Wait more than once", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())

			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})
	})

	Describe("Signal", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			stdoutBuffer := gbytes.NewBuffer()
			processIO = func() (io.Reader, io.Writer, io.Writer, bool) {
				return nil, stdoutBuffer, nil, false
			}

			processSpec := &specs.Process{
				Args: []string{"/bin/sh", "-c", `
					trap 'exit 42' TERM

					while true; do
					  echo 'sleeping'
					  sleep 1
					done
				`},
				Cwd: "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())

			Eventually(stdoutBuffer).Should(gbytes.Say("sleeping"))
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("should forward signals to the process", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())

			Expect(proc.Signal(syscall.SIGTERM)).To(Succeed())

			status := make(chan int)
			go func() {
				exit, err := proc.Wait()
				Expect(err).NotTo(HaveOccurred())
				status <- exit
			}()

			Eventually(status, 5*time.Second).Should(Receive(BeEquivalentTo(42)))
		})
	})

	Describe("GetProcess", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			processSpec := &specs.Process{
				Args: []string{"/bin/sleep", "30"},
				Cwd:  "/",
			}

			_, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("execs a process in the container", func() {
			proc, err := cnerd.GetProcess(testLogger, containerID, processID)
			Expect(err).NotTo(HaveOccurred())

			Expect(proc.ID()).To(Equal(processID))
		})

		Context("when the container does not exist", func() {
			JustBeforeEach(func() {
				cnerd.Delete(testLogger, containerID)
			})

			It("fails", func() {
				_, err := cnerd.GetProcess(testLogger, containerID, processID)
				Expect(err).To(MatchError(ContainSubstring("not found")))
			})
		})

		Context("when the process does not exist", func() {
			It("fails", func() {
				_, err := cnerd.GetProcess(testLogger, containerID, "not-existing-process-id")
				Expect(err).To(MatchError(ContainSubstring("process not-existing-process-id not found in container %s", containerID)))
			})
		})
	})

	Describe("GetTask", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets a backing process object containing the task for a given container ID", func() {
			task, err := cnerd.GetTask(testLogger, containerID)
			Expect(err).NotTo(HaveOccurred())
			Expect(task.ID()).To(Equal(containerID))
		})

		It("returns an error when the container doesn't exist", func() {
			_, err := cnerd.GetTask(testLogger, "does-not-exist")
			Expect(err).To(MatchError(runcontainerd.ContainerNotFoundError{Handle: "does-not-exist"}))
		})
	})

	Describe("RemoveBundle", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
		})

		It("deletes the containerd container by id", func() {
			Expect(cnerd.RemoveBundle(testLogger, containerID)).To(Succeed())

			containers := listContainers(testConfig.CtrBin, testConfig.Socket)
			Expect(containers).NotTo(ContainSubstring(containerID))
		})
	})

	Describe("Delete", func() {
		var setup deleteSpecSetup

		BeforeEach(func() {
			setup = deleteSpecSetup{}
		})

		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			if testRuntimePolicy.IsAlternative() {
				Expect(syscall.Mount(spec.Root.Path, spec.Root.Path, "", syscall.MS_BIND|syscall.MS_REC, "")).To(Succeed())
				rootfsMounted = true
			}
			Expect(setup.create(func() error {
				return cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)
			})).To(Succeed())
		})

		AfterEach(func() {
			setup.cleanup(func() {
				_, _, err := cnerd.State(testLogger, containerID)
				if err == nil {
					Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())
				} else {
					Expect(err).To(BeAssignableToTypeOf(runcontainerd.TaskNotFoundError{}))
				}
				Expect(cnerd.RemoveBundle(testLogger, containerID)).To(Succeed())
				containers, err := containerdClient.Containers(containerdContext)
				Expect(err).NotTo(HaveOccurred())
				for _, container := range containers {
					Expect(container.ID()).NotTo(Equal(containerID), "RemoveBundle left containerd metadata")
				}
			})
		})

		It("deletes the containerd task by container id", func() {
			Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())

			tasks := listTasks(testConfig.CtrBin, testConfig.Socket)
			Expect(tasks).NotTo(ContainSubstring(containerID))
		})

		Context("with an alternative Runtime v2 shim", func() {
			BeforeEach(func() {
				if !testRuntimePolicy.IsAlternative() {
					Skip("requires an alternative Runtime v2 shim")
				}
			})

			It("deletes a created alternative-runtime task through public TaskService", func() {
				pid, status, err := cnerd.State(testLogger, containerID)
				Expect(err).NotTo(HaveOccurred())
				Expect(pid).NotTo(BeZero())
				Expect(status).To(Equal(string(client.Created)))
				identities, err := captureTaskProcesses(pid, containerID)
				Expect(err).NotTo(HaveOccurred(), "cannot prove pre-delete task runtime process identities")
				fmt.Fprintf(GinkgoWriter, "task %s pre-delete runtime identities: %+v\n", containerID, identities)

				Expect(cnerd.Delete(testLogger, containerID)).To(Succeed())

				tasks := listTasks(testConfig.CtrBin, testConfig.Socket)
				Expect(tasks).NotTo(ContainSubstring(containerID))
				containers := listContainers(testConfig.CtrBin, testConfig.Socket)
				Expect(containers).To(ContainSubstring(containerID))
				Eventually(func() error { return taskProcessesGone(identities, procStat) }, "10s", "100ms").Should(Succeed())

				if cgrouputils.IsCgroup2UnifiedMode() {
					err := os.Remove(filepath.Join("/sys/fs/cgroup", fmt.Sprintf("nerdspace%d", GinkgoParallelProcess()), containerID))
					Expect(err == nil || os.IsNotExist(err)).To(BeTrue())
				}
			})
		})
	})

	Describe("State", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets a nonzero pid and created status before Start", func() {
			pid, status, err := cnerd.State(testLogger, containerID)

			Expect(err).NotTo(HaveOccurred())
			Expect(pid).NotTo(BeZero())
			Expect(status).To(BeEquivalentTo(client.Created))
		})

		It("gets the pid and status of a running task", func() {
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
			pid, status, err := cnerd.State(testLogger, containerID)

			Expect(err).NotTo(HaveOccurred())
			Expect(pid).NotTo(BeZero())
			Expect(status).To(BeEquivalentTo(client.Running))
		})
	})

	Describe("GetContainerPID", func() {
		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID)
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		It("gets the container init process pid", func() {
			containerPid, err := cnerd.GetContainerPID(testLogger, containerID)
			Expect(err).ToNot(HaveOccurred())
			Expect(containerPid).NotTo(BeZero())
			if testRuntimePolicy == runcontainerd.RunscRuntime {
				cmdline, err := os.ReadFile(fmt.Sprintf("/proc/%d/cmdline", containerPid))
				Expect(err).NotTo(HaveOccurred())
				arguments := strings.Split(strings.TrimRight(string(cmdline), "\x00"), "\x00")
				Expect(filepath.Base(arguments[0])).To(Equal("runsc-sandbox"))
				Expect(arguments).To(ContainElement("boot"))
				Expect(arguments).To(ContainElement(containerID))
			} else {
				procls := listProcesses(testConfig.CtrBin, testConfig.Socket, containerID)
				Expect(procls).To(ContainSubstring(strconv.Itoa(int(containerPid))))
			}
		})
	})

	Describe("OOMEvents", func() {
		var (
			processSpec *specs.Process
			events      <-chan *apievents.TaskOOM
		)

		JustBeforeEach(func() {
			spec = generateSpec(containerdContext, containerdClient, containerID, withLinuxResources(specs.LinuxResources{
				Memory: &specs.LinuxMemory{
					Limit: int64ptr(30 * 1024 * 1024),
					Swap:  int64ptr(30 * 1024 * 1024),
				},
			}))
			Expect(cnerd.Create(testLogger, containerID, spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			events = cnerd.OOMEvents(testLogger)
			Expect(cnerd.Start(testLogger, containerID)).To(Succeed())

			proc, err := cnerd.Exec(testLogger, containerID, processID, processSpec, processIO)
			Expect(err).NotTo(HaveOccurred())
			_, err = proc.Wait()
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			cnerd.Delete(testLogger, containerID)
		})

		Context("when OOM does not occur", func() {
			BeforeEach(func() {
				processSpec = &specs.Process{
					Args: []string{"/bin/echo", "hi"},
					Cwd:  "/",
				}
			})

			It("does not report OOM", func() {
				Consistently(events).ShouldNot(Receive())
			})
		})

		Context("when OOM occurs", func() {
			BeforeEach(func() {
				processSpec = &specs.Process{
					Args: []string{"/bin/usemem"},
					Cwd:  "/",
				}
			})

			It("reports OOM", func() {
				var event *apievents.TaskOOM
				Eventually(events, "10s").Should(Receive(&event), "<requesting dmesg>")
				Expect(event.ContainerID).To(Equal(containerID))
			})
		})
	})

	Context("when containerd is not running", func() {
		BeforeEach(func() {
			Expect(containerdProcess.Signal(syscall.SIGTERM)).To(Succeed())
			waitStatus, err := containerdProcess.Wait()
			Expect(err).NotTo(HaveOccurred())
			containerdProcess = nil
			Expect(waitStatus.ExitCode()).To(BeZero())
		})

		It("does not panic when loading the container metadata", func() {
			Expect(func() { cnerd.State(testLogger, "a-random-id") }).NotTo(Panic())
		})
	})

	Describe("BundleIDs", func() {
		AfterEach(func() {
			cnerd.Delete(testLogger, "banana")
			cnerd.Delete(testLogger, "banana2")
		})

		It("returns a filtered list of container handles", func() {
			spec = generateSpec(containerdContext, containerdClient, "banana")
			spec.Annotations = map[string]string{"hello": "potato", "abc": "boohoo"}
			Expect(cnerd.Create(testLogger, "banana", spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())
			spec = generateSpec(containerdContext, containerdClient, "banana2")
			spec.Annotations = map[string]string{"abc": "boohoo"}
			Expect(cnerd.Create(testLogger, "banana2", spec, maximusUID, maximusGID, initProcessIO)).To(Succeed())

			handles, err := cnerd.BundleIDs(
				runcontainerd.ContainerFilter{
					Label:        "hello",
					Value:        "potato",
					ComparisonOp: "==",
				}, runcontainerd.ContainerFilter{
					Label:        "abc",
					Value:        "boohoo",
					ComparisonOp: "==",
				})

			Expect(err).NotTo(HaveOccurred())
			Expect(handles).To(ConsistOf("banana"))
		})
	})
})

func createRootfs(modifyRootfs func(string), perm os.FileMode) string {
	var err error
	tmpDir, err := os.MkdirTemp("", "test-rootfs")
	Expect(err).NotTo(HaveOccurred())
	unpackedRootfs := filepath.Join(tmpDir, "unpacked")
	Expect(os.Mkdir(unpackedRootfs, perm)).To(Succeed())
	runCommand(exec.Command("tar", "xf", os.Getenv("GARDEN_TEST_ROOTFS"), "-C", unpackedRootfs))
	Expect(os.Chmod(tmpDir, perm)).To(Succeed())
	modifyRootfs(unpackedRootfs)
	return unpackedRootfs
}

func generateSpec(context context.Context, client *client.Client, containerID string, opts ...oci.SpecOpts) *specs.Spec {
	spec, err := oci.GenerateSpec(context, client, &containers.Container{ID: containerID}, opts...)
	Expect(err).NotTo(HaveOccurred())
	spec.Process.Args = []string{"/bin/sleep", "999999"}
	spec.Root = &specs.Root{
		Path: createRootfs(func(_ string) {}, 0755),
	}
	spec.Annotations = map[string]string{"container-type": "some-container-type"}

	return spec
}

func withLinuxResources(resources specs.LinuxResources) func(context.Context, oci.Client, *containers.Container, *oci.Spec) error {
	return func(_ context.Context, _ oci.Client, _ *containers.Container, spec *oci.Spec) error {
		spec.Linux.Resources = &resources
		return nil
	}
}

func listContainers(ctr, socket string) string {
	return runCtr(ctr, socket, []string{"containers", "list"})
}

func listTasks(ctr, socket string) string {
	return runCtr(ctr, socket, []string{"tasks", "list"})
}

func listProcesses(ctr, socket, containerID string) string {
	return runCtr(ctr, socket, []string{"tasks", "ps", containerID})
}

func containerLabels(ctr, socket, handle string) map[string]interface{} {
	output := runCtr(ctr, socket, []string{"containers", "info", handle})

	var result map[string]interface{}
	Expect(json.Unmarshal([]byte(output), &result)).To(Succeed())
	return result["Labels"].(map[string]interface{})
}

func runCtr(ctr, socket string, args []string) string {
	defaultArgs := []string{"--address", socket, "--namespace", fmt.Sprintf("nerdspace%d", GinkgoParallelProcess())}
	cmd := exec.Command(ctr, append(defaultArgs, args...)...)

	session, err := gexec.Start(cmd, GinkgoWriter, GinkgoWriter)
	Expect(err).NotTo(HaveOccurred())
	Eventually(session, "10s").Should(gexec.Exit(0))

	return string(session.Out.Contents())
}

func randomInt(min, max int) int {
	return min + randomGenerator.Intn(max-min)
}

func randomString(len int) string {
	bytes := make([]byte, len)
	for i := 0; i < len; i++ {
		bytes[i] = byte(randomInt(65, 90))
	}
	return string(bytes)
}

func int64ptr(i int64) *int64 {
	return &i
}
