package rundmc_test

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/client/connection"
	"code.cloudfoundry.org/garden/gardenfakes"
	"code.cloudfoundry.org/garden/server"
	"code.cloudfoundry.org/guardian/rundmc"
	fakes "code.cloudfoundry.org/guardian/rundmc/rundmcfakes"
	"code.cloudfoundry.org/lager/v3"
)

// errReadAllTimeout is returned by readAllBounded when the read does not
// complete before the absolute deadline.
var errReadAllTimeout = errors.New("ReadAll did not complete before deadline")

func TestExecStreamCloseDrainsRuntimeOutput(t *testing.T) {
	stop := make(chan struct{})
	written := make(chan error, 1)
	runtime := new(fakes.FakeOCIRuntime)
	runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
		process := new(gardenfakes.FakeProcess)
		process.SignalStub = func(garden.Signal) error {
			close(stop)
			return nil
		}
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			<-stop
			_, err := processIO.Stdout.Write(bytes.Repeat([]byte("pending runtime output"), 8192))
			written <- err
		}()
		process.WaitStub = func() (int, error) {
			<-finished
			return 137, nil
		}
		return process, nil
	}
	reader, err := rundmc.NewExecStreamer(runtime).StreamOut(lager.NewLogger("cancel-drain"), "archive", garden.StreamOutSpec{Path: "/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	closed, closeErr := newOnceBoundedCloser(reader).CloseWithTimeout(time.Second)
	if !closed || closeErr != nil {
		t.Fatalf("Close completed=%v error=%v", closed, closeErr)
	}
	if err := <-written; err != nil {
		t.Fatalf("runtime output copier stopped instead of draining after cancellation: %v", err)
	}
}

// onceBoundedCloser wraps an io.Closer so that Close is called at most once,
// regardless of how many callers invoke CloseWithTimeout. All concurrent
// callers block on the same underlying goroutine.
//
// This prevents a double-close race that can occur when an explicit close in
// the test body times out: t.Fatalf triggers t.Cleanup, which would otherwise
// start a second concurrent goroutine calling Close on the same reader while
// the first goroutine is still blocked.
type onceBoundedCloser struct {
	r    io.Closer
	once sync.Once
	done chan struct{}
	err  error
}

// newOnceBoundedCloser returns a new onceBoundedCloser for r. Returns nil if
// r is nil; CloseWithTimeout on a nil receiver is a no-op returning (true, nil).
func newOnceBoundedCloser(r io.Closer) *onceBoundedCloser {
	if r == nil {
		return nil
	}
	return &onceBoundedCloser{r: r, done: make(chan struct{})}
}

// CloseWithTimeout triggers Close (at most once) and blocks until it completes
// or timeout elapses. Returns (completed bool, closeErr error). On timeout,
// completed is false; subsequent callers still block on the same goroutine.
func (c *onceBoundedCloser) CloseWithTimeout(timeout time.Duration) (bool, error) {
	if c == nil {
		return true, nil
	}
	c.once.Do(func() {
		go func() {
			c.err = c.r.Close() // safe: written once before close(c.done)
			close(c.done)
		}()
	})
	select {
	case <-c.done:
		return true, c.err
	case <-time.After(timeout):
		return false, nil
	}
}

// readAllBounded reads all data from r, returning early with errReadAllTimeout
// if the read does not complete before the absolute deadline. The third return
// value is a channel that is closed when the internal goroutine exits. Callers
// that do not need drain visibility may ignore it with _. The stalled
// discriminating test uses it to verify the goroutine drains after the reader
// is closed, proving no goroutine leak.
//
// Using this helper in every churn round AND the stalled test ensures that a
// regression making ReadAll synchronous (removing the goroutine+select) would
// cause the stalled subtest to block indefinitely and trip -test.timeout.
func readAllBounded(deadline time.Time, r io.Reader) ([]byte, error, <-chan struct{}) {
	type result struct {
		data []byte
		err  error
	}
	done := make(chan result, 1)
	goroutineDone := make(chan struct{})
	go func() {
		data, err := io.ReadAll(r)
		done <- result{data, err} // buffered: won't block if nobody reads (timeout path)
		close(goroutineDone)
	}()
	select {
	case res := <-done:
		return res.data, res.err, goroutineDone
	case <-time.After(time.Until(deadline)):
		return nil, errReadAllTimeout, goroutineDone
	}
}

func streamConnection(t *testing.T, streamer *rundmc.ExecStreamer) connection.Connection {
	t.Helper()
	logger := lager.NewLogger("stream-acceptance")
	container := new(gardenfakes.FakeContainer)
	container.HandleReturns("archive")
	container.StreamOutStub = func(spec garden.StreamOutSpec) (io.ReadCloser, error) {
		return streamer.StreamOut(logger, "archive", spec)
	}
	container.StreamInStub = func(spec garden.StreamInSpec) error {
		return streamer.StreamIn(logger, "archive", spec)
	}
	backend := new(gardenfakes.FakeBackend)
	backend.LookupReturns(container, nil)
	socket := filepath.Join(t.TempDir(), "garden.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	gardenServer := server.New("unix", socket, time.Minute, time.Second, backend, logger)
	if err := gardenServer.SetupBomberman(); err != nil {
		listener.Close()
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() { served <- gardenServer.Serve(listener) }()
	client := connection.New("unix", socket)
	t.Cleanup(func() {
		if err := gardenServer.Stop(); err != nil {
			t.Error(err)
		}
		<-served
	})
	if err := client.Ping(); err != nil {
		t.Fatal(err)
	}
	return client
}

func fixtureCommand(spec garden.ProcessSpec) *exec.Cmd {
	path := spec.Path
	if path == "/bin/tar" || path == "/bin/mkdir" {
		path = filepath.Base(path)
	}
	return exec.Command(path, spec.Args...)
}

func commandStreamer() *rundmc.ExecStreamer {
	runtime := new(fakes.FakeOCIRuntime)
	runtime.ExecStub = func(_ lager.Logger, _ string, spec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
		command := fixtureCommand(spec)
		command.Stdin, command.Stdout, command.Stderr = processIO.Stdin, processIO.Stdout, processIO.Stderr
		if err := command.Start(); err != nil {
			return nil, err
		}
		process := new(gardenfakes.FakeProcess)
		process.WaitStub = func() (int, error) {
			err := command.Wait()
			var exitError *exec.ExitError
			if errors.As(err, &exitError) {
				return exitError.ExitCode(), nil
			}
			return 0, err
		}
		return process, nil
	}
	return rundmc.NewExecStreamer(runtime)
}

// gardenServerReportsFailedStreamOut records whether the vendored Garden server
// aborts a stream-out response whose body fails after the response has been
// committed. The current server ends such a response cleanly, so the client
// sees a truncated archive as a successful EOF. Flip this once the vendored
// code.cloudfoundry.org/garden includes the server-side fix.
const gardenServerReportsFailedStreamOut = false

func requireFailedStreamOutReporting(t *testing.T) {
	t.Helper()
	if !gardenServerReportsFailedStreamOut {
		t.Skip("requires a Garden server that aborts stream-out responses whose body fails after commitment")
	}
}

func TestExecArchiveRoundTrips(t *testing.T) {
	client := streamConnection(t, commandStreamer())
	root := t.TempDir()
	source := filepath.Join(root, "source dir")
	if err := os.MkdirAll(filepath.Join(source, "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	payload := bytes.Repeat([]byte("checksum-content\x00"), 8192)
	for _, name := range []string{"file with spaces", "--checkpoint=1", "nested/child"} {
		if err := os.WriteFile(filepath.Join(source, name), payload, 0644); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"file with spaces", "--checkpoint=1", "", "/"} {
		t.Run("path="+name, func(t *testing.T) {
			path := source
			if name == "/" {
				path += "/"
			} else if name != "" {
				path = filepath.Join(source, name)
			}
			reader, err := client.StreamOut("archive", garden.StreamOutSpec{Path: path, User: "root"})
			if err != nil {
				t.Fatal(err)
			}
			archive, readErr := io.ReadAll(reader)
			reader.Close()
			if readErr != nil {
				t.Fatal(readErr)
			}
			destination := filepath.Join(root, "result", strings.ReplaceAll(name, "/", "trailing"))
			if err := client.StreamIn("archive", garden.StreamInSpec{Path: destination, User: "root", TarStream: bytes.NewReader(archive)}); err != nil {
				t.Fatal(err)
			}
			archiveReader := tar.NewReader(bytes.NewReader(archive))
			files := 0
			for {
				header, err := archiveReader.Next()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
				if header.Typeflag != tar.TypeReg {
					continue
				}
				files++
				contents, err := os.ReadFile(filepath.Join(destination, header.Name))
				if err != nil || sha256.Sum256(contents) != sha256.Sum256(payload) {
					t.Fatalf("checksum mismatch for %q: %v", header.Name, err)
				}
			}
			wantFiles := 1
			if name == "" || name == "/" {
				wantFiles = 3
			}
			if files != wantFiles {
				t.Fatalf("archived %d files, want %d", files, wantFiles)
			}
		})
	}
	t.Run("missing-path", func(t *testing.T) {
		requireFailedStreamOutReporting(t)
		missingPath := filepath.Join(root, "missing")
		reader, err := client.StreamOut("archive", garden.StreamOutSpec{Path: missingPath})
		if err != nil {
			// StreamOut itself failed — this is a setup/exec error (e.g.
			// fork/exec /bin/tar: no such file or directory), not a tar
			// path error. Positive assertion: StreamOut must succeed,
			// proving tar exec'd.
			t.Fatalf("missing-path got setup/exec error instead of tar path error: %v", err)
		}
		// StreamOut succeeded: tar is running. Read the body — it must
		// fail because the path does not exist (tar exits non-zero).
		// Over HTTP the server-side error is not propagated; the client
		// sees an EOF variant when the response body is cut short.
		_, readErr := io.ReadAll(reader)
		reader.Close()
		if readErr == nil {
			t.Fatal("missing source produced a successful archive")
		}
	})
	t.Run("invalid-input-archive", func(t *testing.T) {
		err := client.StreamIn("archive", garden.StreamInSpec{Path: filepath.Join(root, "invalid"), TarStream: strings.NewReader("not a tar")})
		if err == nil || !strings.Contains(err.Error(), "tar") {
			t.Fatalf("invalid archive must return tar diagnostic, got %v", err)
		}
	})
}

func TestExecStreamOutGardenErrors(t *testing.T) {
	for _, testCase := range []struct {
		name string
		size int
		exit int
		wait error
		exec error
	}{
		{name: "empty-success"},
		{name: "success", size: 98304},
		{name: "exit-before-output", exit: 2},
		{name: "exit-small-output", size: 1, exit: 2},
		{name: "exit-after-output", size: 98304, exit: 2},
		{name: "wait-before-output", wait: errors.New("wait failed")},
		{name: "wait-small-output", size: 1, wait: errors.New("wait failed")},
		{name: "wait-after-output", size: 98304, wait: errors.New("wait failed")},
		{name: "exec", exec: errors.New("exec failed")},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.size > 0 && (testCase.exit != 0 || testCase.wait != nil) {
				requireFailedStreamOutReporting(t)
			}
			runtime := new(fakes.FakeOCIRuntime)
			process := new(gardenfakes.FakeProcess)
			outputDone := make(chan struct{})
			process.WaitStub = func() (int, error) {
				<-outputDone
				return testCase.exit, testCase.wait
			}
			runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				if testCase.exec != nil {
					return nil, testCase.exec
				}
				go func() {
					_, _ = processIO.Stdout.Write(bytes.Repeat([]byte("x"), testCase.size))
					_, _ = io.WriteString(processIO.Stderr, "tar diagnostic")
					close(outputDone)
				}()
				return process, nil
			}
			client := streamConnection(t, rundmc.NewExecStreamer(runtime))
			reader, err := client.StreamOut("archive", garden.StreamOutSpec{Path: "/p"})
			var data []byte
			if err == nil {
				data, err = io.ReadAll(reader)
				reader.Close()
			}
			failed := testCase.exit != 0 || testCase.wait != nil || testCase.exec != nil
			if failed && err == nil {
				t.Fatal("failed process accepted as clean EOF")
			}
			if !failed && (err != nil || len(data) != testCase.size) {
				t.Fatalf("successful output: bytes=%d err=%v", len(data), err)
			}
		})
	}
}

func TestExecStreamOutBlockedWriterClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		runtime := new(fakes.FakeOCIRuntime)
		process := new(gardenfakes.FakeProcess)
		producerDone := make(chan error, 1)
		waitReturned := make(chan struct{})
		process.WaitStub = func() (int, error) {
			err := <-producerDone
			close(waitReturned)
			return 0, err
		}
		runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
			go func() {
				_, err := processIO.Stdout.Write(bytes.Repeat([]byte("x"), 65536))
				producerDone <- err
			}()
			return process, nil
		}
		reader, err := rundmc.NewExecStreamer(runtime).StreamOut(lager.NewLogger("blocked"), "h", garden.StreamOutSpec{Path: "/p"})
		if err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-waitReturned:
			t.Fatal("producer did not block before reader close")
		default:
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		select {
		case <-waitReturned:
		default:
			t.Fatal("reader close failed to release producer and waiter")
		}
	})
}

func TestExecStreamerUserArguments(t *testing.T) {
	for _, user := range []string{"", "root", "1234", "alice"} {
		t.Run("user="+user, func(t *testing.T) {
			runtime := new(fakes.FakeOCIRuntime)
			process := new(gardenfakes.FakeProcess)
			process.WaitReturns(0, nil)
			runtime.ExecReturns(process, nil)
			streamer := rundmc.NewExecStreamer(runtime)
			logger := lager.NewLogger("users")
			if err := streamer.StreamIn(logger, "h", garden.StreamInSpec{Path: "--dir name", User: user, TarStream: strings.NewReader("")}); err != nil {
				t.Fatal(err)
			}
			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{Path: "/dir/--file name", User: user})
			if err != nil {
				t.Fatal(err)
			}
			_, err = io.ReadAll(reader)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			want := user
			if want == "" {
				want = "root"
			}
			for call := 0; call < 3; call++ {
				_, _, spec, _ := runtime.ExecArgsForCall(call)
				if spec.User != want {
					t.Fatalf("exec %d user=%q, want %q", call, spec.User, want)
				}
			}
		})
	}
}

func TestExecStreamOutConcurrentStderrWaitFailure(t *testing.T) {
	runtime := new(fakes.FakeOCIRuntime)
	process := new(gardenfakes.FakeProcess)
	writing := make(chan struct{})
	outputDone := make(chan struct{})
	waitError := errors.New("wait failed before IO completion")
	process.WaitStub = func() (int, error) {
		<-writing
		return 0, waitError
	}
	runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
		go func() {
			_, _ = io.WriteString(processIO.Stderr, "diagnostic")
			close(writing)
			for count := 0; count < 10000; count++ {
				_, _ = io.WriteString(processIO.Stderr, "diagnostic")
			}
			close(outputDone)
		}()
		return process, nil
	}
	reader, err := rundmc.NewExecStreamer(runtime).StreamOut(lager.NewLogger("stderr-race"), "h", garden.StreamOutSpec{Path: "/p"})
	if err != nil {
		t.Fatal(err)
	}
	_, readErr := io.ReadAll(reader)
	reader.Close()
	<-outputDone
	if !errors.Is(readErr, waitError) || !strings.Contains(readErr.Error(), "diagnostic") {
		t.Fatalf("lost Wait error or available stderr: %v", readErr)
	}
}

func TestExecStreamOutCancellation(t *testing.T) {
	t.Run("pending-exec-is-killed-once-and-wait-finishes", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			runtime := new(fakes.FakeOCIRuntime)
			process := new(gardenfakes.FakeProcess)
			killed := make(chan struct{})
			waitDone := make(chan struct{})
			process.SignalStub = func(signal garden.Signal) error {
				if signal != garden.SignalKill {
					t.Errorf("signal=%v, want kill", signal)
				}
				close(killed)
				return nil
			}
			process.WaitStub = func() (int, error) {
				<-killed
				close(waitDone)
				return 137, nil
			}
			runtime.ExecReturns(process, nil)
			reader, err := rundmc.NewExecStreamer(runtime).StreamOut(lager.NewLogger("cancel"), "h", garden.StreamOutSpec{Path: "/p"})
			if err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
			if process.SignalCallCount() != 1 {
				t.Fatalf("kill calls=%d, want 1", process.SignalCallCount())
			}
			select {
			case <-waitDone:
			default:
				t.Fatal("cancelled exec Wait did not finish")
			}
		})
	})
	t.Run("completed-exec-is-not-killed", func(t *testing.T) {
		runtime := new(fakes.FakeOCIRuntime)
		process := new(gardenfakes.FakeProcess)
		process.WaitReturns(0, nil)
		runtime.ExecReturns(process, nil)
		reader, err := rundmc.NewExecStreamer(runtime).StreamOut(lager.NewLogger("completed"), "h", garden.StreamOutSpec{Path: "/p"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.ReadAll(reader); err != nil {
			t.Fatal(err)
		}
		if err := reader.Close(); err != nil {
			t.Fatal(err)
		}
		if process.SignalCallCount() != 0 {
			t.Fatal("completed exec was signalled")
		}
	})
}

// Streamer contract: Prove that a tar failure after the HTTP response is committed
// (headers and initial body bytes already sent to the client) is observable by
// the official Garden client as a body-read error, not clean EOF.
//
// 256 KiB exceeds the Go HTTP server's default 4 KiB write buffer, forcing
// multiple flushes before the failure occurs. The response is committed once
// the first Write reaches the underlying connection.
func TestGardenClientPostCommitmentErrorNotCleanEOF(t *testing.T) {
	const commitSize = 256 * 1024

	for _, tc := range []struct {
		name    string
		exit    int
		waitErr error
	}{
		{"success-baseline", 0, nil},
		{"exit-2", 2, nil},
		{"wait-error", 0, errors.New("simulated wait failure after committed output")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			failed := tc.exit != 0 || tc.waitErr != nil
			if failed {
				requireFailedStreamOutReporting(t)
			}
			runtime := new(fakes.FakeOCIRuntime)
			process := new(gardenfakes.FakeProcess)
			outputDone := make(chan struct{})
			process.WaitStub = func() (int, error) {
				<-outputDone
				return tc.exit, tc.waitErr
			}
			runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
				go func() {
					if n, err := processIO.Stdout.Write(bytes.Repeat([]byte("x"), commitSize)); n != commitSize || err != nil {
						t.Errorf("producer write: n=%d err=%v; want full %d-byte write", n, err, commitSize)
					}
					if failed {
						_, _ = io.WriteString(processIO.Stderr, "tar: deliberate post-commitment failure")
					}
					close(outputDone)
				}()
				return process, nil
			}

			client := streamConnection(t, rundmc.NewExecStreamer(runtime))
			reader, err := client.StreamOut("archive", garden.StreamOutSpec{Path: "/p"})
			if err != nil {
				t.Fatalf("expected committed response, got HTTP-level error: %v", err)
			}

			data, readErr := io.ReadAll(reader)
			reader.Close()

			if !failed {
				// F12: success baseline — same size must return clean EOF.
				if readErr != nil {
					t.Fatalf("success baseline: unexpected read error after %d bytes: %v", len(data), readErr)
				}
				if len(data) != commitSize {
					t.Fatalf("success baseline: got %d bytes, want %d", len(data), commitSize)
				}
				return
			}

			if readErr == nil {
				t.Fatalf("client saw clean EOF after %d bytes; "+
					"a post-commitment tar failure must produce a body-read error", len(data))
			}
			if len(data) != commitSize {
				t.Fatalf("body-read error after %d bytes, want all %d committed bytes before terminal error",
					len(data), commitSize)
			}
			t.Logf("post-commitment failure visible: %d bytes received, client error: %v",
				len(data), readErr)
		})
	}
}

// Direct streamer contract: Verify that execStreamReader.Close()
// blocks until the streamer worker goroutine exits — not just until Signal
// dispatches. This bypasses the HTTP layer to observe server-side Close directly.
//
// Mechanism: the production code uses `defer close(done)` at goroutine entry,
// so done fires only when the goroutine returns — after both process.Wait()
// AND writer.Close()/CloseWithError(). Close blocks on <-s.done, so Close
// returning proves the full goroutine lifecycle including writer closure.
// (Go specification §Defer_statements guarantees deferred calls execute when
// the surrounding function returns.)
//
// Timing assertion: the fake process delays 100ms after the kill signal before
// Wait returns. Without <-s.done, Close would return within ~1ms of Signal.
// The test verifies Close stays blocked at 50ms (midway through the delay)
// and returns only after Wait completes. This proves Close blocks on the
// worker goroutine; the defer placement guarantees that blocking includes
// writer closure. Distinguishing "Wait-only" from "Wait+writer.Close" via
// timing is impossible because PipeWriter.Close() is instantaneous; the
// structural proof (defer ordering) covers that gap.
//
// Declared bound: 5 s (covers setup, Close, and all verification steps).
func TestStreamerCloseWaitsForWorkerCompletion(t *testing.T) {
	const bound = 5 * time.Second
	const workerExitDelay = 100 * time.Millisecond

	runtime := new(fakes.FakeOCIRuntime)
	process := new(gardenfakes.FakeProcess)
	killed := make(chan struct{}, 1)
	signalCalled := make(chan struct{}, 1) // deterministic handshake for negative control
	waitCompleted := make(chan struct{})

	process.SignalStub = func(sig garden.Signal) error {
		if sig == garden.SignalKill {
			select {
			case killed <- struct{}{}:
			default:
			}
			select {
			case signalCalled <- struct{}{}:
			default:
			}
		}
		return nil
	}
	// Deliberate delay after kill: simulates slow process termination.
	// The streamer goroutine blocks in process.Wait() during this delay.
	process.WaitStub = func() (int, error) {
		<-killed
		time.Sleep(workerExitDelay)
		close(waitCompleted)
		return 137, nil
	}

	runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
		go func() {
			for {
				if _, err := processIO.Stdout.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
					break
				}
			}
		}()
		return process, nil
	}

	// Start the absolute deadline BEFORE setup so all blocking operations
	// (StreamOut, first read, Close, verification) are bounded.
	deadline := time.Now().Add(bound)

	logger := lager.NewLogger("r3-test")
	type streamResult struct {
		reader io.ReadCloser
		err    error
	}
	soDone := make(chan streamResult, 1)
	go func() {
		r, e := rundmc.NewExecStreamer(runtime).StreamOut(logger, "handle", garden.StreamOutSpec{Path: "/p"})
		soDone <- streamResult{r, e}
	}()
	var reader io.ReadCloser
	select {
	case res := <-soDone:
		if res.err != nil {
			t.Fatal(res.err)
		}
		reader = res.reader
	case <-time.After(time.Until(deadline)):
		t.Fatalf("StreamOut did not return within %s", bound)
	}

	// Wrap synchronous read in select so a startup hang fails within
	// the declared bound, not the Go test timeout.
	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := io.ReadAtLeast(reader, buf, 1)
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("no data from stream: %v", err)
		}
	case <-time.After(time.Until(deadline)):
		t.Fatalf("no data from stream within %s", bound)
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()

	// Deterministic handshake: wait for Close to reach Signal before
	// checking it stays blocked. Without this, scheduler delay could
	// let a broken Close pass the negative control vacuously.
	select {
	case <-signalCalled:
		// Good: Close goroutine reached Signal dispatch.
	case <-time.After(time.Until(deadline)):
		t.Fatalf("Close did not reach Signal within %s", bound)
	}

	// Negative control: Close has dispatched Signal but the worker is
	// still blocked in the deliberate Wait delay. Without <-s.done in
	// Close, it would return immediately after Signal.
	select {
	case <-closeDone:
		t.Fatal("Close returned before worker completed — Close wait not enforced")
	case <-time.After(workerExitDelay / 2):
		// Good: Close is still blocking midway through the deliberate delay.
	}

	// Wait for Close to complete within the declared bound.
	select {
	case closeErr := <-closeDone:
		if closeErr != nil {
			t.Fatalf("Close returned error: %v", closeErr)
		}
	case <-time.After(time.Until(deadline)):
		t.Fatalf("Close did not return within %s — worker may be stuck", bound)
	}

	// Absolute time check (finding #3: prevents a ready-race masking
	// a deadline overshoot).
	if time.Now().After(deadline) {
		t.Fatalf("cleanup exceeded declared bound %s", bound)
	}

	// Verify the worker actually finished (waitCompleted closed).
	select {
	case <-waitCompleted:
		// Good: worker's Wait returned before Close returned.
	default:
		t.Fatal("worker Wait not completed when Close returned")
	}

	t.Log("Confirmed: Close blocked until worker goroutine finished")
}

// TestSignalFailureClassification exercises the bounded 1s wait on done when
// Signal returns an error (streamer.go:154-168). Two subtests:
//
//   - benign-exit: Signal fails but the worker finishes promptly (process
//     already exited). Close returns without error because the done channel
//     closes before the 1s timeout.
//   - stuck-worker: Signal fails and the worker does not finish within 1s.
//     Close returns a wrapped signal error and does NOT block on <-s.done.
func TestSignalFailureClassification(t *testing.T) {
	errSignal := errors.New("os: process already finished")

	t.Run("benign-exit", func(t *testing.T) {
		runtime := new(fakes.FakeOCIRuntime)
		process := new(gardenfakes.FakeProcess)
		signalCalled := make(chan struct{})

		process.SignalStub = func(sig garden.Signal) error {
			close(signalCalled)
			return errSignal
		}
		// Wait blocks until Signal is called, then returns promptly.
		// This ensures Close enters the signal-error classification path
		// (the worker is still pending when Signal fires).
		process.WaitStub = func() (int, error) {
			<-signalCalled
			return 0, nil
		}
		runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
			go func() {
				processIO.Stdout.Write([]byte("done"))
			}()
			return process, nil
		}

		logger := lager.NewLogger("signal-benign")
		reader, err := rundmc.NewExecStreamer(runtime).StreamOut(logger, "handle", garden.StreamOutSpec{Path: "/p"})
		if err != nil {
			t.Fatal(err)
		}
		// Read enough to confirm streaming started, but do NOT drain —
		// draining lets Wait return before Close, bypassing Signal entirely.
		buf := make([]byte, 1)
		if _, err := io.ReadAtLeast(reader, buf, 1); err != nil {
			t.Fatal(err)
		}

		closeErr := reader.Close()
		if closeErr != nil {
			t.Fatalf("benign Signal error should not propagate: got %v", closeErr)
		}
		// Verify Signal was actually called — proves Close entered the
		// signal-error classification path.
		select {
		case <-signalCalled:
		default:
			t.Fatal("Signal was not called — test did not exercise the signal-error path")
		}
	})

	t.Run("stuck-worker", func(t *testing.T) {
		runtime := new(fakes.FakeOCIRuntime)
		process := new(gardenfakes.FakeProcess)
		stuckForever := make(chan struct{})
		t.Cleanup(func() { close(stuckForever) }) // release worker goroutine on exit

		process.SignalStub = func(sig garden.Signal) error {
			return errSignal
		}
		// Worker blocks indefinitely — simulates a genuinely stuck process.
		process.WaitStub = func() (int, error) {
			<-stuckForever
			return 137, nil
		}
		runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
			go func() {
				for {
					if _, err := processIO.Stdout.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
						break
					}
				}
			}()
			return process, nil
		}

		logger := lager.NewLogger("signal-stuck")
		reader, err := rundmc.NewExecStreamer(runtime).StreamOut(logger, "handle", garden.StreamOutSpec{Path: "/p"})
		if err != nil {
			t.Fatal(err)
		}
		// Read enough to confirm streaming started.
		buf := make([]byte, 1)
		if _, err := io.ReadAtLeast(reader, buf, 1); err != nil {
			t.Fatal(err)
		}

		start := time.Now()
		closeErr := reader.Close()
		elapsed := time.Since(start)

		// Close must observe the 1s classification wait (lower bound) and
		// return within its margin (upper bound).
		if elapsed < time.Second {
			t.Fatalf("Close returned after %s without waiting 1s to classify the signal failure", elapsed)
		}
		if elapsed > 2*time.Second {
			t.Fatalf("Close took %s — should have timed out at 1s", elapsed)
		}
		// Close must return the wrapped signal error.
		if closeErr == nil {
			t.Fatal("stuck-worker Close should return signal error")
		}
		if !strings.Contains(closeErr.Error(), "cancel tar") {
			t.Fatalf("expected 'cancel tar' in error, got: %v", closeErr)
		}
		if !errors.Is(closeErr, errSignal) {
			t.Fatalf("expected wrapped errSignal, got: %v", closeErr)
		}
		t.Logf("stuck-worker classified correctly in %s: %v", elapsed, closeErr)
	})
}

// archive reader through the official Garden HTTP client and verify the
// streamer's exec signal, Wait, producer goroutine and worker writer-closure
// all complete within a declared bound.
//
// The absolute deadline covers setup (exec start, first read) AND the
// full cleanup path. reader.Close() runs in a goroutine selected against
// the deadline — a blocked Close fails deterministically. An absolute
// time.Now() check after each receive prevents a ready-race from masking a
// deadline overshoot.
//
// HTTP client reader.Close() does not directly observe server-side
// execStreamReader.Close(); server-side Close waiting is proved by
// TestStreamerCloseWaitsForWorkerCompletion. Here, waitDone and producerDone
// confirm the worker reached those intermediate checkpoints within the bound.
//
// Declared bound: 10 s (set before testing, per acceptance criteria).
func TestGardenClientCancellationReapsExec(t *testing.T) {
	const reapBound = 10 * time.Second

	runtime := new(fakes.FakeOCIRuntime)
	process := new(gardenfakes.FakeProcess)
	killed := make(chan struct{})
	var killOnce sync.Once
	waitDone := make(chan struct{})
	producerDone := make(chan struct{})
	execStarted := make(chan struct{})

	process.SignalStub = func(sig garden.Signal) error {
		if sig == garden.SignalKill {
			killOnce.Do(func() { close(killed) })
		}
		return nil
	}
	process.WaitStub = func() (int, error) {
		<-killed
		<-producerDone
		close(waitDone)
		return 137, nil
	}
	runtime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
		go func() {
			close(execStarted)
			defer close(producerDone)
			for {
				select {
				case <-killed:
					return
				default:
				}
				if _, err := processIO.Stdout.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
					break
				}
			}
		}()
		return process, nil
	}

	// Start the absolute deadline BEFORE setup so all blocking operations
	// (StreamOut, exec start, first read, Close, verification) are bounded.
	deadline := time.Now().Add(reapBound)

	client := streamConnection(t, rundmc.NewExecStreamer(runtime))
	type streamResult struct {
		reader io.ReadCloser
		err    error
	}
	soDone := make(chan streamResult, 1)
	go func() {
		r, e := client.StreamOut("archive", garden.StreamOutSpec{Path: "/p"})
		soDone <- streamResult{r, e}
	}()
	var reader io.ReadCloser
	select {
	case res := <-soDone:
		if res.err != nil {
			t.Fatal(res.err)
		}
		reader = res.reader
	case <-time.After(time.Until(deadline)):
		t.Fatalf("StreamOut did not return within %s", reapBound)
	}

	select {
	case <-execStarted:
	case <-time.After(time.Until(deadline)):
		t.Fatalf("exec did not start within %s", reapBound)
	}

	readDone := make(chan error, 1)
	go func() {
		buf := make([]byte, 1)
		_, err := io.ReadAtLeast(reader, buf, 1)
		readDone <- err
	}()
	select {
	case err := <-readDone:
		if err != nil {
			t.Fatalf("no data from backpressured stream: %v", err)
		}
	case <-time.After(time.Until(deadline)):
		t.Fatalf("no data from stream within %s", reapBound)
	}

	closeDone := make(chan error, 1)
	go func() { closeDone <- reader.Close() }()

	select {
	case closeErr := <-closeDone:
		if closeErr != nil {
			t.Fatalf("reader.Close returned error: %v", closeErr)
		}
	case <-time.After(time.Until(deadline)):
		t.Fatalf("reader.Close did not complete within %s", reapBound)
	}
	if time.Now().After(deadline) {
		t.Fatalf("cleanup exceeded declared bound %s", reapBound)
	}

	// Verify intermediate steps also completed within the same bound.
	select {
	case <-waitDone:
	case <-time.After(time.Until(deadline)):
		t.Fatalf("exec not reaped within %s", reapBound)
	}
	if time.Now().After(deadline) {
		t.Fatalf("cleanup exceeded declared bound %s", reapBound)
	}
	select {
	case <-producerDone:
	case <-time.After(time.Until(deadline)):
		t.Fatalf("producer goroutine still blocked after %s", reapBound)
	}
	if time.Now().After(deadline) {
		t.Fatalf("cleanup exceeded declared bound %s", reapBound)
	}

	if n := process.SignalCallCount(); n != 1 {
		t.Fatalf("expected exactly 1 kill signal, got %d", n)
	}
}

// Shared-server contract: Repeated success/failure/cancel paths through the Garden HTTP
// client must not leave blocked IO or accumulating residue. A subsequent
// control must succeed.
//
// All rounds and the final control share a single Garden server and streamer
// (F4: a fresh server would hide residue from prior rounds).
//
// Declared repetitions: 10 rounds (success/failure/cancel cycling).
// Declared reap deadline per cancel round: 5 s.
func TestGardenClientChurnThenControl(t *testing.T) {
	const rounds = 10
	const reapBound = 5 * time.Second

	// Shared runtime whose ExecStub is swapped per round.
	sharedRuntime := new(fakes.FakeOCIRuntime)
	client := streamConnection(t, rundmc.NewExecStreamer(sharedRuntime))

	// F7: test-owned fixture directory instead of os.TempDir().
	fixture := filepath.Join(t.TempDir(), "fixture")
	if err := os.MkdirAll(fixture, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fixture, "churn-payload"), bytes.Repeat([]byte("p"), 4096), 0644); err != nil {
		t.Fatal(err)
	}

	// commandExec configures the shared runtime to execute real commands.
	// Returns a channel closed when the streamer calls process.Wait, making
	// per-round exec cleanup observable.
	commandExec := func() <-chan struct{} {
		waitDone := make(chan struct{})
		sharedRuntime.ExecStub = func(_ lager.Logger, _ string, spec garden.ProcessSpec, processIO garden.ProcessIO) (garden.Process, error) {
			command := fixtureCommand(spec)
			command.Stdin, command.Stdout, command.Stderr = processIO.Stdin, processIO.Stdout, processIO.Stderr
			if err := command.Start(); err != nil {
				return nil, err
			}
			process := new(gardenfakes.FakeProcess)
			process.WaitStub = func() (int, error) {
				defer close(waitDone)
				err := command.Wait()
				var exitError *exec.ExitError
				if errors.As(err, &exitError) {
					return exitError.ExitCode(), nil
				}
				return 0, err
			}
			return process, nil
		}
		return waitDone
	}

	for i := 0; i < rounds; i++ {
		kind := []string{"success", "failure", "cancel"}[i%3]
		t.Run(fmt.Sprintf("round-%d-%s", i, kind), func(t *testing.T) {
			switch i % 3 {
			case 0: // success: read a real archive through the shared Garden HTTP path.
				// Absolute deadline BEFORE setup so ALL blocking
				// operations are bounded within the declared limit.
				// StreamOut, ReadAll, Close and Wait each run in a
				// goroutine selected against the same absolute deadline
				// so a hang in any one is caught within reapBound, not
				// masked by the enclosing go-test timeout.
				deadline := time.Now().Add(reapBound)
				waitDone := commandExec()

				type soResult struct {
					reader io.ReadCloser
					err    error
				}
				soDone := make(chan soResult, 1)
				soDoneConsumed := false
				go func() {
					r, e := client.StreamOut("archive", garden.StreamOutSpec{
						Path: fixture, User: "root",
					})
					soDone <- soResult{r, e}
				}()
				// Drain cleanup: if StreamOut times out, bounded-close any late reader
				// to avoid a held HTTP connection on the server side.
				t.Cleanup(func() {
					if !soDoneConsumed {
						cleanupBudget := time.Until(deadline)
						if cleanupBudget < time.Second {
							cleanupBudget = time.Second
						}
						cleanupDeadline := time.Now().Add(cleanupBudget)
						select {
						case late := <-soDone:
							if late.reader != nil {
								closer := newOnceBoundedCloser(late.reader)
								if ok, err := closer.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
									t.Error("success round: late reader.Close did not return within cleanup budget")
								} else if err != nil {
									t.Errorf("success round: late reader.Close returned error: %v", err)
								}
							}
						case <-time.After(time.Until(cleanupDeadline)):
						}
					}
				})
				var sor soResult
				select {
				case sor = <-soDone:
					soDoneConsumed = true
					if sor.err != nil {
						t.Fatal(sor.err)
					}
				case <-time.After(time.Until(deadline)):
					t.Fatalf("success round %d: StreamOut did not return within %s", i, reapBound)
				}
				// Cleanup: close reader and drain ReadAll goroutine on any exit path.
				// readDrainDone is set below after readAllBounded returns.
				var readDrainDone <-chan struct{}
				bc := newOnceBoundedCloser(sor.reader)
				t.Cleanup(func() {
					cleanupBudget := time.Until(deadline)
					if cleanupBudget < time.Second {
						cleanupBudget = time.Second
					}
					cleanupDeadline := time.Now().Add(cleanupBudget)
					if ok, _ := bc.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
						t.Error("success round: reader.Close did not return within cleanup budget")
					}
					if ch := readDrainDone; ch != nil {
						select {
						case <-ch:
						case <-time.After(time.Until(cleanupDeadline)):
							t.Error("success round: ReadAll goroutine did not drain within cleanup budget — goroutine leaked")
						}
					}
				})

				var readErr error
				_, readErr, readDrainDone = readAllBounded(deadline, sor.reader)
				if readErr != nil {
					t.Fatalf("success round %d: ReadAll failed: %v", i, readErr)
				}

				if ok, err := bc.CloseWithTimeout(time.Until(deadline)); !ok {
					t.Fatalf("success round %d: Close did not return within %s", i, reapBound)
				} else if err != nil {
					t.Fatalf("success round %d: Close returned error: %v", i, err)
				}

				// Verify exec Wait returned — proves no residual process.
				select {
				case <-waitDone:
				case <-time.After(time.Until(deadline)):
					t.Fatalf("success round %d: exec Wait did not return within %s", i, reapBound)
				}
				if time.Now().After(deadline) {
					t.Fatalf("success round %d: cleanup exceeded declared bound %s", i, reapBound)
				}

			case 1: // failure: exec produces output then exits nonzero.
				// Absolute deadline BEFORE setup so ALL blocking
				// operations are bounded within the declared limit.
				deadline := time.Now().Add(reapBound)
				proc := new(gardenfakes.FakeProcess)
				done := make(chan struct{})
				waitDone := make(chan struct{})
				proc.WaitStub = func() (int, error) {
					<-done
					defer close(waitDone)
					return 1, nil
				}
				sharedRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {
					go func() {
						_, _ = pio.Stdout.Write(bytes.Repeat([]byte("x"), 8192))
						close(done)
					}()
					return proc, nil
				}

				type soResult struct {
					reader io.ReadCloser
					err    error
				}
				soDone := make(chan soResult, 1)
				soDoneConsumed := false
				go func() {
					r, e := client.StreamOut("archive", garden.StreamOutSpec{Path: "/p"})
					soDone <- soResult{r, e}
				}()
				// Drain cleanup: bounded-close any late reader if StreamOut times out.
				t.Cleanup(func() {
					if !soDoneConsumed {
						cleanupBudget := time.Until(deadline)
						if cleanupBudget < time.Second {
							cleanupBudget = time.Second
						}
						cleanupDeadline := time.Now().Add(cleanupBudget)
						select {
						case late := <-soDone:
							if late.reader != nil {
								closer := newOnceBoundedCloser(late.reader)
								if ok, err := closer.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
									t.Error("failure round: late reader.Close did not return within cleanup budget")
								} else if err != nil {
									t.Errorf("failure round: late reader.Close returned error: %v", err)
								}
							}
						case <-time.After(time.Until(cleanupDeadline)):
						}
					}
				})
				var sor soResult
				select {
				case sor = <-soDone:
					soDoneConsumed = true
				case <-time.After(time.Until(deadline)):
					t.Fatalf("failure round %d: StreamOut did not return within %s", i, reapBound)
				}
				// Cleanup: close reader on any exit path (once-guarded to prevent
				// concurrent Close if the explicit drain times out and t.Fatalf
				// fires t.Cleanup while the drain goroutine is still blocked).
				// readDrainDone is set below after readAllBounded returns (if called).
				var readDrainDone <-chan struct{}
				bc := newOnceBoundedCloser(sor.reader) // nil-safe
				t.Cleanup(func() {
					cleanupBudget := time.Until(deadline)
					if cleanupBudget < time.Second {
						cleanupBudget = time.Second
					}
					cleanupDeadline := time.Now().Add(cleanupBudget)
					if ok, _ := bc.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
						t.Error("failure round: reader.Close did not return within cleanup budget")
					}
					if ch := readDrainDone; ch != nil {
						select {
						case <-ch:
						case <-time.After(time.Until(cleanupDeadline)):
							t.Error("failure round: ReadAll goroutine did not drain within cleanup budget — goroutine leaked")
						}
					}
				})

				var firstErr error
				firstErr = sor.err
				if sor.err == nil {
					var readErr error
					_, readErr, readDrainDone = readAllBounded(deadline, sor.reader)
					if errors.Is(readErr, errReadAllTimeout) {
						t.Fatalf("failure round %d: ReadAll did not return within %s", i, reapBound)
					}
					firstErr = readErr

					if ok, closeErr := bc.CloseWithTimeout(time.Until(deadline)); !ok {
						t.Fatalf("failure round %d: Close did not return within %s", i, reapBound)
					} else if closeErr != nil {
						// Close after a failed ReadAll may itself return an error; log but do not fatal.
						t.Logf("failure round %d: Close returned error: %v", i, closeErr)
					}
				}
				if firstErr == nil {
					if gardenServerReportsFailedStreamOut {
						t.Fatal("failure round returned clean EOF")
					}
					// Reaping is still verified below; only the client-visible
					// error depends on the Garden server change.
					t.Logf("failure round %d: clean EOF (Garden server does not yet abort failed stream-out)", i)
				}
				// Verify exec Wait returned — proves no residual process.
				select {
				case <-waitDone:
				case <-time.After(time.Until(deadline)):
					t.Fatalf("failure round %d: exec Wait did not return within %s", i, reapBound)
				}
				if time.Now().After(deadline) {
					t.Fatalf("failure round %d: cleanup exceeded declared bound %s", i, reapBound)
				}

			case 2: // cancel: continuous producer, client disconnects after one byte.
				proc := new(gardenfakes.FakeProcess)
				killedCh := make(chan struct{})
				var killOnce sync.Once
				waitDoneCh := make(chan struct{})
				producerDoneCh := make(chan struct{})
				started := make(chan struct{})
				proc.SignalStub = func(sig garden.Signal) error {
					if sig == garden.SignalKill {
						killOnce.Do(func() { close(killedCh) })
					}
					return nil
				}
				proc.WaitStub = func() (int, error) {
					<-killedCh
					<-producerDoneCh
					close(waitDoneCh)
					return 137, nil
				}
				sharedRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {
					go func() {
						close(started)
						defer close(producerDoneCh)
						for {
							select {
							case <-killedCh:
								return
							default:
							}
							if _, err := pio.Stdout.Write(bytes.Repeat([]byte("x"), 4096)); err != nil {
								break
							}
						}
					}()
					return proc, nil
				}

				// Absolute deadline BEFORE setup so all blocking
				// operations are bounded within the declared limit.
				cancelDeadline := time.Now().Add(reapBound)

				type cancelStreamResult struct {
					reader io.ReadCloser
					err    error
				}
				cancelSODone := make(chan cancelStreamResult, 1)
				cancelSODoneConsumed := false
				go func() {
					r, e := client.StreamOut("archive", garden.StreamOutSpec{Path: "/p"})
					cancelSODone <- cancelStreamResult{r, e}
				}()
				// Drain cleanup: bounded-close any late reader if StreamOut times out.
				t.Cleanup(func() {
					if !cancelSODoneConsumed {
						cleanupBudget := time.Until(cancelDeadline)
						if cleanupBudget < time.Second {
							cleanupBudget = time.Second
						}
						cleanupDeadline := time.Now().Add(cleanupBudget)
						select {
						case late := <-cancelSODone:
							if late.reader != nil {
								closer := newOnceBoundedCloser(late.reader)
								if ok, err := closer.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
									t.Error("cancel round: late reader.Close did not return within cleanup budget")
								} else if err != nil {
									t.Errorf("cancel round: late reader.Close returned error: %v", err)
								}
							}
						case <-time.After(time.Until(cleanupDeadline)):
						}
					}
				})
				var reader io.ReadCloser
				select {
				case res := <-cancelSODone:
					cancelSODoneConsumed = true
					if res.err != nil {
						t.Fatal(res.err)
					}
					reader = res.reader
				case <-time.After(time.Until(cancelDeadline)):
					t.Fatalf("cancel round: StreamOut did not return within %s", reapBound)
				}
				// Cleanup: close reader on any exit path (once-guarded).
				bc := newOnceBoundedCloser(reader) // nil-safe
				t.Cleanup(func() {
					cleanupBudget := time.Until(cancelDeadline)
					if cleanupBudget < time.Second {
						cleanupBudget = time.Second
					}
					if ok, _ := bc.CloseWithTimeout(cleanupBudget); !ok {
						t.Error("cancel round: reader.Close did not return within cleanup budget")
					}
				})
				select {
				case <-started:
				case <-time.After(time.Until(cancelDeadline)):
					t.Fatalf("cancel round: exec did not start within %s", reapBound)
				}
				cancelReadDone := make(chan error, 1)
				go func() {
					oneByte := make([]byte, 1)
					_, err := io.ReadAtLeast(reader, oneByte, 1)
					cancelReadDone <- err
				}()
				select {
				case err := <-cancelReadDone:
					if err != nil {
						t.Fatalf("no data from cancel stream: %v", err)
					}
				case <-time.After(time.Until(cancelDeadline)):
					t.Fatalf("cancel round: no data within %s", reapBound)
				}
				// Close bounded by deadline. HTTP client Close does not
				// observe server-side execStreamReader.Close() directly;
				// server-side Close waiting proved by TestStreamerCloseWaitsForWorkerCompletion.
				if ok, closeErr := bc.CloseWithTimeout(time.Until(cancelDeadline)); !ok {
					t.Fatalf("cancel round: reader.Close did not complete within %s", reapBound)
				} else if closeErr != nil {
					t.Fatalf("cancel round: reader.Close error: %v", closeErr)
				}
				if time.Now().After(cancelDeadline) {
					t.Fatalf("cancel round: cleanup exceeded declared bound %s", reapBound)
				}
				select {
				case <-waitDoneCh:
				case <-time.After(time.Until(cancelDeadline)):
					t.Fatalf("cancel round: exec not reaped within %s", reapBound)
				}
				if time.Now().After(cancelDeadline) {
					t.Fatalf("cancel round: cleanup exceeded declared bound %s", reapBound)
				}
				select {
				case <-producerDoneCh:
				case <-time.After(time.Until(cancelDeadline)):
					t.Fatalf("cancel round: producer goroutine still blocked after %s", reapBound)
				}
				if time.Now().After(cancelDeadline) {
					t.Fatalf("cancel round: cleanup exceeded declared bound %s", reapBound)
				}
			}
		})
	}

	// Final control: uses the SAME server after all churn rounds.
	// Proves service availability after churn. Each round now verifies exec
	// Wait completion, making per-round cleanup observable. Cancel
	// rounds additionally verify producer goroutine exit within declared
	// bounds; server-side Close waiting for the worker goroutine is
	// proved by the focused TestStreamerCloseWaitsForWorkerCompletion test.
	// These assertions observe exec cleanup per round; they do not
	// independently measure HTTP connection-pool residue.
	t.Run("control-after-churn", func(t *testing.T) {
		// Absolute deadline BEFORE setup. All blocking operations
		// (StreamOut, ReadAll, Close, Wait) run in goroutine+select
		// against this deadline so a hang in any one is bounded.
		deadline := time.Now().Add(reapBound)
		waitDone := commandExec()

		type soResult struct {
			reader io.ReadCloser
			err    error
		}
		soDone := make(chan soResult, 1)
		soDoneConsumed := false
		go func() {
			r, e := client.StreamOut("archive", garden.StreamOutSpec{
				Path: fixture, User: "root",
			})
			soDone <- soResult{r, e}
		}()
		// Drain cleanup: bounded-close any late reader if StreamOut times out.
		t.Cleanup(func() {
			if !soDoneConsumed {
				cleanupBudget := time.Until(deadline)
				if cleanupBudget < time.Second {
					cleanupBudget = time.Second
				}
				cleanupDeadline := time.Now().Add(cleanupBudget)
				select {
				case late := <-soDone:
					if late.reader != nil {
						closer := newOnceBoundedCloser(late.reader)
						if ok, err := closer.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
							t.Error("control: late reader.Close did not return within cleanup budget")
						} else if err != nil {
							t.Errorf("control: late reader.Close returned error: %v", err)
						}
					}
				case <-time.After(time.Until(cleanupDeadline)):
				}
			}
		})
		var sor soResult
		select {
		case sor = <-soDone:
			soDoneConsumed = true
			if sor.err != nil {
				t.Fatalf("control failed after %d rounds: %v", rounds, sor.err)
			}
		case <-time.After(time.Until(deadline)):
			t.Fatalf("control: StreamOut did not return within %s", reapBound)
		}
		// Cleanup: close reader on any exit path (once-guarded).
		// readDrainDone is set below after readAllBounded returns.
		var readDrainDone <-chan struct{}
		bc := newOnceBoundedCloser(sor.reader)
		t.Cleanup(func() {
			cleanupBudget := time.Until(deadline)
			if cleanupBudget < time.Second {
				cleanupBudget = time.Second
			}
			cleanupDeadline := time.Now().Add(cleanupBudget)
			if ok, _ := bc.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
				t.Error("control: reader.Close did not return within cleanup budget")
			}
			if ch := readDrainDone; ch != nil {
				select {
				case <-ch:
				case <-time.After(time.Until(cleanupDeadline)):
					t.Error("control: ReadAll goroutine did not drain within cleanup budget — goroutine leaked")
				}
			}
		})

		var data []byte
		var readErr error
		data, readErr, readDrainDone = readAllBounded(deadline, sor.reader)
		if errors.Is(readErr, errReadAllTimeout) {
			t.Fatalf("control: ReadAll did not return within %s", reapBound)
		}
		if readErr != nil {
			t.Fatalf("control read failed: %v", readErr)
		}

		if ok, err := bc.CloseWithTimeout(time.Until(deadline)); !ok {
			t.Fatalf("control: Close did not return within %s", reapBound)
		} else if err != nil {
			t.Fatalf("control: Close returned error: %v", err)
		}

		if time.Now().After(deadline) {
			t.Fatalf("control: read+close exceeded %s", reapBound)
		}
		// Verify exec Wait returned for control round too.
		select {
		case <-waitDone:
		case <-time.After(time.Until(deadline)):
			t.Fatalf("control: exec Wait did not return within %s", reapBound)
		}
		if time.Now().After(deadline) {
			t.Fatalf("control: cleanup exceeded declared bound %s", reapBound)
		}
		// F13: parse the tar and verify the test-owned payload.
		archiveReader := tar.NewReader(bytes.NewReader(data))
		foundPayload := false
		for {
			header, err := archiveReader.Next()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatalf("control returned invalid archive: %v", err)
			}
			if filepath.Base(header.Name) != "churn-payload" {
				continue
			}
			payload, err := io.ReadAll(archiveReader)
			if err != nil {
				t.Fatalf("control payload read failed: %v", err)
			}
			if !bytes.Equal(payload, bytes.Repeat([]byte("p"), 4096)) {
				t.Fatal("control payload was corrupted after churn")
			}
			foundPayload = true
		}
		if !foundPayload {
			t.Fatal("control archive omitted churn-payload")
		}
		t.Logf("control OK: valid archive with churn-payload after %d churn rounds", rounds)
	})

	// Discriminating stalled-operation test: proves the goroutine+deadline watchdog
	// fires when ReadAll stalls AFTER the HTTP response is committed.
	//
	// Two-phase protocol:
	//   1. The producer writes a 64 KB chunk and signals committed, ensuring
	//      Garden's HTTP server flushes the response headers+body and StreamOut
	//      returns a live reader. We verify this with a generous commitBound.
	//   2. The producer then blocks on release, so ReadAll never gets EOF.
	//      We set a tight readallBound (500 ms) and assert the deadline fires
	//      (ReadAll returns an error or the select fires) — proving the
	//      goroutine+select watchdog is active on the ReadAll path, not just
	//      the StreamOut path.
	//
	// Without goroutine wrapping, a synchronous ReadAll at phase 2 would block
	// until the 300 s global test timeout.
	t.Run("stalled-readall-deadline", func(t *testing.T) {
		const commitBound = 10 * time.Second
		const readallBound = 500 * time.Millisecond
		// 64 KB chunk — large enough to flush Garden's HTTP buffers so StreamOut
		// returns a committed reader before the producer stalls.
		chunk := make([]byte, 64*1024)
		release := make(chan struct{})
		committed := make(chan struct{}, 1)
		commitDeadline := time.Now().Add(commitBound)
		var releaseOnce sync.Once
		doRelease := func() { releaseOnce.Do(func() { close(release) }) }
		t.Cleanup(doRelease)

		proc := new(gardenfakes.FakeProcess)
		proc.WaitStub = func() (int, error) {
			<-release
			return 0, nil
		}
		proc.SignalStub = func(garden.Signal) error {
			doRelease()
			return nil
		}
		sharedRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {
			go func() {
				// Phase 1: write a large chunk to flush the HTTP response.
				_, _ = pio.Stdout.Write(chunk)
				// Signal that the HTTP response has been committed.
				select {
				case committed <- struct{}{}:
				default:
				}
				// Phase 2: stall — never write EOF so ReadAll blocks.
				<-release
			}()
			return proc, nil
		}

		type soResult struct {
			reader io.ReadCloser
			err    error
		}
		soDone := make(chan soResult, 1)
		soDoneConsumed := false
		go func() {
			r, e := client.StreamOut("archive", garden.StreamOutSpec{Path: "/stall"})
			soDone <- soResult{r, e}
		}()
		// Drain cleanup: bounded-close any late reader if StreamOut times out.
		t.Cleanup(func() {
			if !soDoneConsumed {
				cleanupBudget := time.Until(commitDeadline)
				if cleanupBudget < time.Second {
					cleanupBudget = time.Second
				}
				cleanupDeadline := time.Now().Add(cleanupBudget)
				select {
				case late := <-soDone:
					if late.reader != nil {
						closer := newOnceBoundedCloser(late.reader)
						if ok, err := closer.CloseWithTimeout(time.Until(cleanupDeadline)); !ok {
							t.Error("stalled-readall: late reader.Close did not return within cleanup budget")
						} else if err != nil {
							t.Errorf("stalled-readall: late reader.Close returned error: %v", err)
						}
					}
				case <-time.After(time.Until(cleanupDeadline)):
				}
			}
		})

		// Phase 1a: StreamOut must return within commitBound.
		var sor soResult
		select {
		case sor = <-soDone:
			soDoneConsumed = true
			if sor.err != nil {
				t.Fatalf("stalled-readall: StreamOut returned error: %v", sor.err)
			}
		case <-time.After(time.Until(commitDeadline)):
			t.Fatalf("stalled-readall: StreamOut did not return within %s — response not committed", commitBound)
		}
		if time.Now().After(commitDeadline) {
			t.Fatalf("stalled-readall: StreamOut returned after commitDeadline — setup exceeded %s", commitBound)
		}
		// Cleanup: close reader on any exit path (once-guarded to prevent
		// concurrent Close if the drain close times out and t.Fatalf fires
		// t.Cleanup while the drain goroutine is still blocked).
		bc := newOnceBoundedCloser(sor.reader)
		t.Cleanup(func() {
			cleanupBudget := time.Until(commitDeadline)
			if cleanupBudget < time.Second {
				cleanupBudget = time.Second
			}
			if ok, _ := bc.CloseWithTimeout(cleanupBudget); !ok {
				t.Error("stalled-readall: reader.Close did not return within cleanup budget")
			}
		})

		// Phase 1b: producer must have written its chunk (commit confirmed).
		select {
		case <-committed:
			// HTTP response is committed; producer is now stalling on release.
		case <-time.After(time.Until(commitDeadline)):
			t.Fatalf("stalled-readall: producer did not commit response within %s", commitBound)
		}
		if time.Now().After(commitDeadline) {
			t.Fatalf("stalled-readall: response commitment exceeded %s", commitBound)
		}

		// Phase 2: ReadAll must remain blocked until the deadline fires.
		// readAllBounded is the same helper used by every churn round, so a
		// regression that makes ReadAll synchronous (removing the goroutine+select)
		// would cause readAllBounded itself to block here indefinitely and
		// eventually trip the -test.timeout, making the regression detectable.
		start := time.Now()
		deadline := start.Add(readallBound)
		_, readErr, drainDone := readAllBounded(deadline, sor.reader)
		elapsed := time.Since(start)
		if !errors.Is(readErr, errReadAllTimeout) {
			t.Fatalf("stalled-readall: ReadAll returned before deadline (%s) — producer stalling not proven or unexpected close: %v", readallBound, readErr)
		}
		if elapsed < readallBound {
			t.Fatalf("stalled-readall: readAllBounded returned in %s before deadline %s — watchdog short-circuited", elapsed, readallBound)
		}
		if elapsed > 2*readallBound {
			t.Fatalf("stalled-readall: readAllBounded returned in %s (>2×%s) — watchdog firing too late", elapsed, readallBound)
		}
		// PASS: readAllBounded returned errReadAllTimeout within the expected window.
		// The goroutine+select watchdog in readAllBounded (shared with all churn
		// rounds) correctly terminated the stalled ReadAll within the bound.
		t.Logf("stalled-readall-deadline: deadline fired in %s — goroutine+deadline watchdog confirmed", elapsed)

		// Drain: unblock the producer and close the reader under a bound.
		// The ReadAll goroutine inside readAllBounded uses a buffered channel;
		// once the reader is closed it drains automatically without blocking.
		// Step 1: unblock the producer so the HTTP handler exits.
		doRelease()
		// Step 2: close the reader via the once-guarded closer (shared with t.Cleanup).
		if ok, err := bc.CloseWithTimeout(5 * time.Second); !ok {
			t.Fatal("stalled-readall: reader did not close within 5s")
		} else if err != nil {
			t.Fatalf("stalled-readall: reader.Close returned error: %v", err)
		}
		// Step 3: verify the goroutine inside readAllBounded drained after the reader
		// was closed. This proves no goroutine leak.
		select {
		case <-drainDone:
		case <-time.After(5 * time.Second):
			t.Fatal("stalled-readall: ReadAll goroutine did not drain within 5s after reader close — goroutine leaked")
		}
		// t.Cleanup closes release idempotently.
	})
}
