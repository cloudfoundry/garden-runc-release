//go:build linux

package guardiancmd

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client/connection"
	"code.cloudfoundry.org/garden/server"
	"code.cloudfoundry.org/guardian/gardener"
	"code.cloudfoundry.org/guardian/gardener/gardenerfakes"
	"code.cloudfoundry.org/guardian/rundmc"
	"code.cloudfoundry.org/guardian/rundmc/depot"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
	"code.cloudfoundry.org/guardian/rundmc/runcontainerd/runcontainerdfakes"
	"code.cloudfoundry.org/guardian/rundmc/runrunc"
	"code.cloudfoundry.org/lager/v3"
	cgroup1stats "github.com/containerd/cgroups/v3/cgroup1/stats"
	cgroup2stats "github.com/containerd/cgroups/v3/cgroup2/stats"
	containersapi "github.com/containerd/containerd/api/services/containers/v1"
	tasksapi "github.com/containerd/containerd/api/services/tasks/v1"
	containertypes "github.com/containerd/containerd/api/types"
	tasktypes "github.com/containerd/containerd/api/types/task"
	"github.com/containerd/containerd/v2/pkg/namespaces"
	"github.com/containerd/containerd/v2/plugins"
	"github.com/containerd/typeurl/v2"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Runtime-type policy tests. The image-backed pea and streamer
// decisions depend on RuntimePolicy.IsAlternative(), resolved once at
// startup via ResolveRuntimePolicy. These cases cover the real CLI
// configuration space: empty (default runc), explicit stock runc-v2, an
// alternative runtime (runsc-v1/gVisor), and unsupported types.
//
// Regression guard: if someone changes the condition (e.g. drops the
// empty-string check), the stock runc case would incorrectly enable
// image-backed peas and ExecStreamer, breaking pea creation for all stock runc deployments.
// If the unsupported-type guard is removed, unknown runtimes would
// silently inherit all runsc-specific workarounds.

type runtimeWiringFactory struct {
	*LinuxFactory
}

func (f runtimeWiringFactory) WireCPUCgrouper() (rundmc.CPUCgrouper, error) {
	return nil, nil
}

func TestContainerizerWithoutContainerdUsesStockStrategies(t *testing.T) {
	for _, runtimeType := range []string{"", "io.containerd.runc.v2", "io.containerd.runsc.v1", "io.containerd.kata.v2"} {
		t.Run(runtimeType, func(t *testing.T) {
			cmd := &CommonCommand{}
			cmd.Containers.Dir = t.TempDir()
			cmd.Containerd.RuntimeType = runtimeType
			factory := runtimeWiringFactory{LinuxFactory: &LinuxFactory{config: cmd}}
			containerizer, _, err := cmd.wireContainerizer(lager.NewLogger("runtime-wiring"), factory, nil, nil, 1, depot.NetworkDepot{}, nil)
			if err != nil {
				t.Fatal(err)
			}
			wired := reflect.ValueOf(containerizer).Elem()
			if got := wired.FieldByName("runtime").Elem().Type().String(); got != "*runrunc.RunRunc" {
				t.Errorf("runtime = %s, want stock *runrunc.RunRunc", got)
			}
			if got := wired.FieldByName("streamer").Elem().Type().String(); got != "*rundmc.NstarStreamer" {
				t.Errorf("streamer = %s, want stock *rundmc.NstarStreamer", got)
			}
			if wired.FieldByName("imageBackedPeas").Bool() {
				t.Error("stock runtime enabled runsc image-backed peas without a containerd socket")
			}
		})
	}
}

func TestContainerizerWithContainerdUsesValidatedStrategies(t *testing.T) {
	for _, test := range []struct {
		runtimeType     string
		streamer        string
		imageBackedPeas bool
		wantError       bool
	}{
		{streamer: "*rundmc.NstarStreamer"},
		{runtimeType: "io.containerd.runc.v2", streamer: "*rundmc.NstarStreamer"},
		{runtimeType: "io.containerd.runsc.v1", streamer: "*rundmc.ExecStreamer", imageBackedPeas: true},
		{runtimeType: "io.containerd.kata.v2", wantError: true},
		{runtimeType: "io.containerd.runsc.v2", wantError: true},
	} {
		for _, containerdProcesses := range []bool{false, true} {
			t.Run(test.runtimeType+"/processes="+strconv.FormatBool(containerdProcesses), func(t *testing.T) {
				socketDir, err := os.MkdirTemp("", "gdn-policy-")
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { os.RemoveAll(socketDir) })
				cmd := &CommonCommand{}
				cmd.Containers.Dir = t.TempDir()
				cmd.Containerd.Socket = filepath.Join(socketDir, "containerd.sock")
				cmd.Containerd.RuntimeType = test.runtimeType
				cmd.Containerd.UseContainerdForProcesses = containerdProcesses
				listener, err := net.Listen("unix", cmd.Containerd.Socket)
				if err != nil {
					t.Fatal(err)
				}
				fixture := &runtimeMetricsRPCFixture{calls: make(map[string]int)}
				rpcServer := grpc.NewServer(grpc.UnaryInterceptor(fixture.intercept))
				serveResult := make(chan error, 1)
				go func() { serveResult <- rpcServer.Serve(listener) }()
				t.Cleanup(func() {
					rpcServer.Stop()
					if err := <-serveResult; err != nil && err != grpc.ErrServerStopped {
						t.Error(err)
					}
				})
				factory := runtimeWiringFactory{LinuxFactory: &LinuxFactory{config: cmd}}
				containerizer, cleaner, err := cmd.wireContainerizer(lager.NewLogger("runtime-wiring"), factory, nil, nil, 1, depot.NetworkDepot{}, nil)
				wantError := ""
				if test.wantError {
					wantError = "unsupported containerd runtime type"
				} else if test.imageBackedPeas && !containerdProcesses {
					wantError = "requires --use-containerd-for-processes"
				}
				if wantError != "" {
					if err == nil || !strings.Contains(err.Error(), wantError) || !strings.Contains(err.Error(), test.runtimeType) {
						t.Fatalf("construction error = %v, want actionable %q rejection for %q", err, wantError, test.runtimeType)
					}
					if containerizer != nil || cleaner != nil {
						t.Error("rejected runtime returned partially wired components")
					}
				} else {
					if err != nil {
						t.Fatal(err)
					}
					wired := reflect.ValueOf(containerizer).Elem()
					if got := wired.FieldByName("runtime").Elem().Type().String(); got != "*runcontainerd.RunContainerd" {
						t.Errorf("runtime = %s, want *runcontainerd.RunContainerd", got)
					}
					if got := wired.FieldByName("streamer").Elem().Type().String(); got != test.streamer {
						t.Errorf("streamer = %s, want %s", got, test.streamer)
					}
					if got := wired.FieldByName("imageBackedPeas").Bool(); got != test.imageBackedPeas {
						t.Errorf("imageBackedPeas = %v, want %v", got, test.imageBackedPeas)
					}
					creator := wired.FieldByName("peaCreator").Elem()
					if got := creator.Type().String(); got != "*peas.PeaCreator" {
						t.Fatalf("peaCreator = %s, want *peas.PeaCreator", got)
					}
					if got := creator.Elem().FieldByName("GroupedRunsc").Bool(); got != test.imageBackedPeas {
						t.Errorf("PeaCreator.GroupedRunsc = %v, want %v", got, test.imageBackedPeas)
					}
				}
				fixture.mutex.Lock()
				defer fixture.mutex.Unlock()
				if len(fixture.calls) != 0 || len(fixture.violations) != 0 {
					t.Errorf("construction made unexpected containerd RPCs: %v, %v", fixture.calls, fixture.violations)
				}
			})
		}
	}
}

func TestResolveRuntimePolicyIsAlternative(t *testing.T) {
	tests := []struct {
		name        string
		runtimeType string
		want        bool
		wantErr     bool
	}{
		{
			name:        "empty string defaults to stock runc",
			runtimeType: "",
			want:        false,
		},
		{
			name:        "explicit stock runc-v2 is not alternative",
			runtimeType: "io.containerd.runc.v2",
			want:        false,
		},
		{
			name:        "runsc-v1 is an alternative runtime (gVisor)",
			runtimeType: "io.containerd.runsc.v1",
			want:        true,
		},
		{
			// unsupported types are rejected at WireContainerd.
			// ResolveRuntimePolicy returns an error, and the zero-value
			// policy (StockRuntime) is not alternative — safe default.
			name:        "unknown runtime is rejected with error",
			runtimeType: "io.containerd.kata.v2",
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := runcontainerd.ResolveRuntimePolicy(tt.runtimeType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveRuntimePolicy(%q) succeeded; want error for unsupported runtime", tt.runtimeType)
				}
				// Even on error, the returned policy must not activate alternative paths
				if policy.IsAlternative() {
					t.Errorf("rejected policy.IsAlternative() = true; unsupported runtimes must not activate alternative-runtime paths")
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRuntimePolicy(%q) error: %v", tt.runtimeType, err)
			}
			if got := policy.IsAlternative(); got != tt.want {
				t.Errorf("ResolveRuntimePolicy(%q).IsAlternative() = %v, want %v", tt.runtimeType, got, tt.want)
			}
		})
	}
}

func TestWireContainerdMetricsThroughGarden(t *testing.T) {
	const handle = "factory-metrics-container"
	v1, err := typeurl.MarshalAnyToProto(&cgroup1stats.Metrics{
		CPU: &cgroup1stats.CPUStat{Usage: &cgroup1stats.CPUUsage{Total: 123456789, User: 98765432, Kernel: 24691357}},
		Memory: &cgroup1stats.MemoryStat{
			Cache: 71, RSS: 91, TotalCache: 73, TotalRSS: 97,
			HierarchicalMemoryLimit: 4096, Usage: &cgroup1stats.MemoryEntry{Usage: 157},
		},
		Pids: &cgroup1stats.PidsStat{Current: 3, Limit: 41},
	})
	if err != nil {
		t.Fatal(err)
	}
	v2, err := typeurl.MarshalAnyToProto(&cgroup2stats.Metrics{
		CPU: &cgroup2stats.CPUStat{UsageUsec: 123457, UserUsec: 98765, SystemUsec: 24692},
		Memory: &cgroup2stats.MemoryStat{
			Anon: 131, File: 67, InactiveFile: 19, SwapUsage: 23, UsageLimit: 8192,
		},
		Pids: &cgroup2stats.PidsStat{Current: 5, Limit: 43},
	})
	if err != nil {
		t.Fatal(err)
	}
	stock := gardener.StatsContainerMetrics{
		CPU:    garden.ContainerCPUStat{Usage: 919191919, User: 818181818, System: 101010101},
		Memory: garden.ContainerMemoryStat{TotalUsageTowardLimit: 717171, Rss: 616161, Cache: 515151},
		Pid:    garden.ContainerPidStat{Current: 17, Max: 101},
		Age:    73*time.Second + 29*time.Nanosecond,
	}
	wantV1 := gardener.StatsContainerMetrics{
		CPU: garden.ContainerCPUStat{Usage: 123456789, User: 98765432, System: 24691357},
		Memory: garden.ContainerMemoryStat{
			Cache: 71, Rss: 91, TotalCache: 73, TotalRss: 97, Anon: 97, File: 73,
			HierarchicalMemoryLimit: 4096, TotalUsageTowardLimit: 157,
		},
		Pid: garden.ContainerPidStat{Current: 3, Max: 41},
	}
	wantV2 := gardener.StatsContainerMetrics{
		CPU: garden.ContainerCPUStat{Usage: 123457000, User: 98765000, System: 24692000},
		Memory: garden.ContainerMemoryStat{
			Anon: 131, File: 67, Rss: 131, TotalRss: 131, Cache: 67, TotalCache: 67,
			InactiveFile: 19, TotalInactiveFile: 19, Swap: 23, TotalSwap: 23,
			HierarchicalMemoryLimit: 8192, TotalUsageTowardLimit: 179,
		},
		Pid: garden.ContainerPidStat{Current: 5, Max: 43},
	}
	tests := []struct {
		name         string
		runtimeType  string
		payload      *anypb.Any
		containerErr error
		taskErr      error
		metricsErr   error
		want         gardener.StatsContainerMetrics
		wantError    string
		wantStock    bool
		wantRPCs     map[string]int
	}{
		{
			name: "empty runtime retains injected runc statser", payload: v2, want: stock, wantStock: true,
		},
		{
			name: "explicit runc retains injected runc statser", runtimeType: plugins.RuntimeRuncV2,
			payload: v2, want: stock, wantStock: true,
		},
		{
			name: "runsc decodes v1 nanoseconds", runtimeType: "io.containerd.runsc.v1",
			payload: v1, want: wantV1,
			wantRPCs: map[string]int{"containers.Get": 2, "tasks.Get": 1, "tasks.Metrics": 1},
		},
		{
			name: "runsc converts v2 microseconds", runtimeType: "io.containerd.runsc.v1",
			payload: v2, want: wantV2,
			wantRPCs: map[string]int{"containers.Get": 2, "tasks.Get": 1, "tasks.Metrics": 1},
		},
		{
			name: "unknown payload returns Garden error", runtimeType: "io.containerd.runsc.v1",
			payload:   &anypb.Any{TypeUrl: "example.invalid/unknown-metrics", Value: []byte{1}},
			wantError: "unsupported metrics type",
			wantRPCs:  map[string]int{"containers.Get": 2, "tasks.Get": 1, "tasks.Metrics": 1},
		},
		{
			name: "malformed v1 returns Garden error", runtimeType: "io.containerd.runsc.v1",
			payload: &anypb.Any{TypeUrl: v1.TypeUrl, Value: []byte{0xff}}, wantError: "decode v1 metrics",
			wantRPCs: map[string]int{"containers.Get": 2, "tasks.Get": 1, "tasks.Metrics": 1},
		},
		{
			name: "malformed v2 returns Garden error", runtimeType: "io.containerd.runsc.v1",
			payload: &anypb.Any{TypeUrl: v2.TypeUrl, Value: []byte{0xff}}, wantError: "decode v2 metrics",
			wantRPCs: map[string]int{"containers.Get": 2, "tasks.Get": 1, "tasks.Metrics": 1},
		},
		{
			name: "container lookup error reaches Garden", runtimeType: "io.containerd.runsc.v1",
			containerErr: status.Error(codes.Unavailable, "container lookup failed"), wantError: "container lookup failed",
			wantRPCs: map[string]int{"containers.Get": 1},
		},
		{
			name: "task lookup error reaches Garden", runtimeType: "io.containerd.runsc.v1",
			taskErr: status.Error(codes.Unavailable, "task lookup failed"), wantError: "task lookup failed",
			wantRPCs: map[string]int{"containers.Get": 1, "tasks.Get": 1},
		},
		{
			name: "task metrics error reaches Garden", runtimeType: "io.containerd.runsc.v1",
			metricsErr: status.Error(codes.Unavailable, "task metrics failed"), wantError: "task metrics failed",
			wantRPCs: map[string]int{"containers.Get": 1, "tasks.Get": 1, "tasks.Metrics": 1},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			socketDir, err := os.MkdirTemp("", "gdn-metrics-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := os.RemoveAll(socketDir); err != nil {
					t.Error(err)
				}
			})
			fixture := &runtimeMetricsRPCFixture{
				handle: handle, createdAt: time.Now().Add(-3 * time.Hour), payload: test.payload,
				containerErr: test.containerErr, taskErr: test.taskErr, metricsErr: test.metricsErr,
				calls: make(map[string]int),
			}
			containerdSocket := filepath.Join(socketDir, "containerd.sock")
			listener, err := net.Listen("unix", containerdSocket)
			if err != nil {
				t.Fatal(err)
			}
			rpcServer := grpc.NewServer(grpc.UnaryInterceptor(fixture.intercept))
			containersapi.RegisterContainersServer(rpcServer, &runtimeMetricsContainersServer{fixture: fixture})
			tasksapi.RegisterTasksServer(rpcServer, &runtimeMetricsTasksServer{fixture: fixture})
			serveResult := make(chan error, 1)
			go func() { serveResult <- rpcServer.Serve(listener) }()
			t.Cleanup(func() {
				rpcServer.Stop()
				if err := <-serveResult; err != nil && err != grpc.ErrServerStopped {
					t.Error(err)
				}
			})

			log := lager.NewLogger("factory-metrics-test")
			stockStatser := &runcontainerdfakes.FakeStatser{}
			stockStatser.StatsReturns(stock, nil)
			factory := &LinuxFactory{config: &CommonCommand{}}
			factory.config.Containerd.Socket = containerdSocket
			factory.config.Containerd.RuntimeType = test.runtimeType
			factory.config.Containerd.UseContainerdForProcesses = true
			manager, _, _, _, _, _, err := factory.WireContainerd(nil, nil, func(runrunc.PidGetter) *runrunc.Execer {
				return nil
			}, stockStatser, log, nil, nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			containerizer := &gardenerfakes.FakeContainerizer{}
			containerizer.MetricsStub = func(logger lager.Logger, id string) (gardener.ActualContainerMetrics, error) {
				stats, err := manager.Stats(logger, id)
				return gardener.ActualContainerMetrics{StatsContainerMetrics: stats}, err
			}
			backend := &gardener.Gardener{
				Logger: log, Containerizer: containerizer, Volumizer: &gardenerfakes.FakeVolumizer{},
				ContainerNetworkMetricsProvider: gardener.NewNoopContainerNetworkMetricsProvider(),
			}
			gardenSocket := filepath.Join(socketDir, "garden.sock")
			gardenServer := server.New("unix", gardenSocket, 0, time.Second, backend, log)
			if err := gardenServer.SetupBomberman(); err != nil {
				t.Fatal(err)
			}
			gardenListener, err := gardenServer.Listen()
			if err != nil {
				t.Fatal(err)
			}
			gardenServeResult := make(chan error, 1)
			go func() { gardenServeResult <- gardenServer.Serve(gardenListener) }()
			t.Cleanup(func() {
				if err := gardenServer.Stop(); err != nil {
					t.Error(err)
				}
				if err := <-gardenServeResult; err != nil {
					t.Error(err)
				}
			})
			before := time.Now()
			got, err := connection.New("unix", gardenSocket).Metrics(handle)
			after := time.Now()
			if test.wantError != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantError) {
					t.Errorf("Garden Metrics error = %v, want containing %q", err, test.wantError)
				}
				if !reflect.DeepEqual(got, garden.Metrics{}) {
					t.Errorf("failed Garden Metrics returned nonzero metrics: %+v", got)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				want := garden.Metrics{CPUStat: test.want.CPU, MemoryStat: test.want.Memory, PidStat: test.want.Pid, Age: test.want.Age}
				if !test.wantStock {
					if got.Age < before.Sub(fixture.createdAt) || got.Age > after.Sub(fixture.createdAt) {
						t.Errorf("age %v does not derive from RPC creation time %v", got.Age, fixture.createdAt)
					}
					want.Age = got.Age
				}
				if !reflect.DeepEqual(got, want) {
					t.Errorf("Garden Metrics = %+v, want %+v", got, want)
				}
			}
			wantStockCalls := 0
			if test.wantStock {
				wantStockCalls = 1
			}
			if got := stockStatser.StatsCallCount(); got != wantStockCalls {
				t.Errorf("injected statser calls = %d, want %d", got, wantStockCalls)
			}
			if stockStatser.StatsCallCount() > 0 {
				_, id := stockStatser.StatsArgsForCall(0)
				if id != handle {
					t.Errorf("injected statser container ID = %q, want %q", id, handle)
				}
			}
			fixture.mutex.Lock()
			defer fixture.mutex.Unlock()
			wantRPCs := test.wantRPCs
			if wantRPCs == nil {
				wantRPCs = map[string]int{}
			}
			if !reflect.DeepEqual(fixture.calls, wantRPCs) {
				t.Errorf("containerd RPCs = %v, want %v", fixture.calls, wantRPCs)
			}
			if len(fixture.violations) != 0 {
				t.Errorf("containerd RPC contract violations: %v", fixture.violations)
			}
		})
	}
}

type runtimeMetricsRPCFixture struct {
	handle       string
	createdAt    time.Time
	payload      *anypb.Any
	containerErr error
	taskErr      error
	metricsErr   error
	mutex        sync.Mutex
	calls        map[string]int
	violations   []string
}

func (fixture *runtimeMetricsRPCFixture) intercept(ctx context.Context, request interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	fixture.mutex.Lock()
	defer fixture.mutex.Unlock()
	namespace, _ := namespaces.Namespace(ctx)
	if namespace != "garden" {
		fixture.violations = append(fixture.violations, "unexpected namespace: "+namespace)
		return nil, status.Error(codes.InvalidArgument, "expected garden namespace")
	}
	switch info.FullMethod {
	case "/containerd.services.containers.v1.Containers/Get":
		fixture.calls["containers.Get"]++
		if request.(*containersapi.GetContainerRequest).ID != fixture.handle {
			fixture.violations = append(fixture.violations, "unexpected containers.Get ID")
			return nil, status.Error(codes.InvalidArgument, "unexpected container ID")
		}
	case "/containerd.services.tasks.v1.Tasks/Get":
		fixture.calls["tasks.Get"]++
		taskRequest := request.(*tasksapi.GetRequest)
		if taskRequest.ContainerID != fixture.handle || taskRequest.ExecID != "" {
			fixture.violations = append(fixture.violations, "unexpected tasks.Get ID")
			return nil, status.Error(codes.InvalidArgument, "unexpected task ID")
		}
	case "/containerd.services.tasks.v1.Tasks/Metrics":
		fixture.calls["tasks.Metrics"]++
		if !reflect.DeepEqual(request.(*tasksapi.MetricsRequest).Filters, []string{"id==" + fixture.handle}) {
			fixture.violations = append(fixture.violations, "unexpected metrics filter")
			return nil, status.Error(codes.InvalidArgument, "unexpected metrics filter")
		}
	default:
		fixture.violations = append(fixture.violations, "unexpected RPC: "+info.FullMethod)
		return nil, status.Error(codes.Unimplemented, "unexpected RPC")
	}
	return handler(ctx, request)
}

type runtimeMetricsContainersServer struct {
	containersapi.UnimplementedContainersServer
	fixture *runtimeMetricsRPCFixture
}

func (rpc *runtimeMetricsContainersServer) Get(context.Context, *containersapi.GetContainerRequest) (*containersapi.GetContainerResponse, error) {
	if rpc.fixture.containerErr != nil {
		return nil, rpc.fixture.containerErr
	}
	return &containersapi.GetContainerResponse{Container: &containersapi.Container{
		ID: rpc.fixture.handle, CreatedAt: timestamppb.New(rpc.fixture.createdAt),
		Runtime: &containersapi.Container_Runtime{Name: "io.containerd.runsc.v1"},
	}}, nil
}

type runtimeMetricsTasksServer struct {
	tasksapi.UnimplementedTasksServer
	fixture *runtimeMetricsRPCFixture
}

func (rpc *runtimeMetricsTasksServer) Get(context.Context, *tasksapi.GetRequest) (*tasksapi.GetResponse, error) {
	if rpc.fixture.taskErr != nil {
		return nil, rpc.fixture.taskErr
	}
	return &tasksapi.GetResponse{Process: &tasktypes.Process{
		ContainerID: rpc.fixture.handle, ID: rpc.fixture.handle, Pid: 42, Status: tasktypes.Status_UNKNOWN,
	}}, nil
}

func (rpc *runtimeMetricsTasksServer) Metrics(context.Context, *tasksapi.MetricsRequest) (*tasksapi.MetricsResponse, error) {
	if rpc.fixture.metricsErr != nil {
		return nil, rpc.fixture.metricsErr
	}
	return &tasksapi.MetricsResponse{Metrics: []*containertypes.Metric{{
		ID: rpc.fixture.handle, Timestamp: timestamppb.Now(), Data: rpc.fixture.payload,
	}}}, nil
}

// WireContainerd must reject unsupported runtime types at startup
// with an actionable error, preventing unknown runtimes from silently
// inheriting runsc-specific workarounds (cleanup7, ExecStreamer, image-backed
// peas,
// ContainerdStatser). This test exercises the real LinuxFactory wiring path.

func TestWireContainerdRejectsUnsupportedRuntime(t *testing.T) {
	unsupported := []string{
		"io.containerd.kata.v2",
		"io.containerd.runsc.v2",
		"custom-runtime",
		"runc",
	}
	for _, rt := range unsupported {
		t.Run(rt, func(t *testing.T) {
			socketDir, err := os.MkdirTemp("", "gdn-reject-")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.RemoveAll(socketDir) })

			factory := &LinuxFactory{config: &CommonCommand{}}
			factory.config.Containerd.Socket = filepath.Join(socketDir, "containerd.sock")
			factory.config.Containerd.RuntimeType = rt

			_, _, _, _, _, _, err = factory.WireContainerd(nil, nil, func(runrunc.PidGetter) *runrunc.Execer {
				return nil
			}, nil, lager.NewLogger("reject-test"), nil, nil, nil)

			if err == nil {
				t.Fatalf("WireContainerd(%q) succeeded; want error for unsupported runtime", rt)
			}
			errMsg := err.Error()
			if !strings.Contains(errMsg, "unsupported") {
				t.Errorf("error %q does not mention 'unsupported'", errMsg)
			}
			if !strings.Contains(errMsg, rt) {
				t.Errorf("error %q does not mention the rejected runtime type %q", errMsg, rt)
			}
		})
	}
}

// Behavior matrix — verifies the complete runtime-type decision
// space at the CLI wiring layer. For each of the four input types (empty,
// explicit runc, runsc, unsupported), verifies that ResolveRuntimePolicy
// produces the expected RuntimePolicy which determines streamer, image-backed
// peas,
// and statser selection. The statser selection is covered by
// TestWireContainerdMetricsThroughGarden above; the Nerd-level decisions
// (strip-Windows, task-opts, cleanup7) are covered by the policy unit tests
// and require Linux for behavioral verification.
//
// The policy is resolved once at startup
// (in WireContainerd) and threaded to all downstream consumers. There is
// no secondary re-resolution from the config string.
//
// Matrix columns:
//
//	policy.IsAlternative() → streamer (ExecStreamer vs NstarStreamer)
//	policy.IsAlternative() → image-backed grouped peas
//	policy.IsAlternative() → statser (ContainerdStatser vs runc events)
//	RunscRuntime           → strip-Windows, cleanup7 (runsc-specific)
//	StockRuntime           → NoNewKeyring, UID/GID task opts (stock-specific)
//	unsupported            → rejected at WireContainerd before any wiring

func TestRuntimePolicyBehaviorMatrix(t *testing.T) {
	tests := []struct {
		name        string
		runtimeType string

		// Expected wiring decisions
		wantIsAlt  bool // drives streamer, image-backed peas, statser
		wantReject bool // unsupported types rejected at WireContainerd
	}{
		{
			name:        "empty (default runc): stock streamer, stock peas, stock statser",
			runtimeType: "",
			wantIsAlt:   false,
		},
		{
			name:        "explicit runc-v2: identical to empty — stock strategies",
			runtimeType: "io.containerd.runc.v2",
			wantIsAlt:   false,
		},
		{
			name:        "runsc-v1: exec streamer, image-backed peas, containerd statser",
			runtimeType: "io.containerd.runsc.v1",
			wantIsAlt:   true,
		},
		{
			name:        "kata-v2: rejected at startup, no wiring decisions",
			runtimeType: "io.containerd.kata.v2",
			wantReject:  true,
		},
		{
			name:        "arbitrary string: rejected at startup",
			runtimeType: "my-custom-runtime",
			wantReject:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := runcontainerd.ResolveRuntimePolicy(tt.runtimeType)

			if tt.wantReject {
				if err == nil {
					t.Fatalf("ResolveRuntimePolicy(%q) = %v, nil; want rejection error", tt.runtimeType, policy)
				}
				// Verify that the zero-value policy on error doesn't activate
				// alternative-runtime paths — safe stock default.
				if policy.IsAlternative() {
					t.Errorf("rejected policy.IsAlternative() = true for %q; unsupported runtimes must not activate alternative-runtime paths", tt.runtimeType)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRuntimePolicy(%q) error: %v", tt.runtimeType, err)
			}

			gotIsAlt := policy.IsAlternative()
			if gotIsAlt != tt.wantIsAlt {
				t.Errorf("policy.IsAlternative() = %v, want %v", gotIsAlt, tt.wantIsAlt)
			}

			// Verify stock identity: empty and explicit runc must produce the same policy
			if tt.runtimeType == "" || tt.runtimeType == "io.containerd.runc.v2" {
				if policy != runcontainerd.StockRuntime {
					t.Errorf("stock runtime type %q resolved to %v, want StockRuntime", tt.runtimeType, policy)
				}
			}

			// Verify runsc-specific behaviors are only asserted for the runsc policy
			if tt.runtimeType == "io.containerd.runsc.v1" {
				if policy != runcontainerd.RunscRuntime {
					t.Errorf("runsc runtime type resolved to %v, want RunscRuntime", policy)
				}
			}
		})
	}
}
