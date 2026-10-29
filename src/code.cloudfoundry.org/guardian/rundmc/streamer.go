package rundmc

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager/v3"
)

//counterfeiter:generate . Streamer
type Streamer interface {
	StreamIn(log lager.Logger, handle string, spec garden.StreamInSpec) error
	StreamOut(log lager.Logger, handle string, spec garden.StreamOutSpec) (io.ReadCloser, error)
}

type NstarStreamer struct {
	runtime OCIRuntime
	nstar   NstarRunner
}

func NewNstarStreamer(runtime OCIRuntime, nstar NstarRunner) *NstarStreamer {
	return &NstarStreamer{runtime: runtime, nstar: nstar}
}

func (s *NstarStreamer) StreamIn(log lager.Logger, handle string, spec garden.StreamInSpec) error {
	state, err := s.runtime.State(log, handle)
	if err != nil {
		return fmt.Errorf("stream-in: pid not found for container")
	}
	if err := s.nstar.StreamIn(log, state.Pid, spec.Path, spec.User, spec.TarStream); err != nil {
		return fmt.Errorf("stream-in: nstar: %s", err)
	}
	return nil
}

func (s *NstarStreamer) StreamOut(log lager.Logger, handle string, spec garden.StreamOutSpec) (io.ReadCloser, error) {
	state, err := s.runtime.State(log, handle)
	if err != nil {
		return nil, fmt.Errorf("stream-out: pid not found for container")
	}
	stream, err := s.nstar.StreamOut(log, state.Pid, spec.Path, spec.User)
	if err != nil {
		return nil, fmt.Errorf("stream-out: nstar: %s", err)
	}
	return stream, nil
}

type ExecStreamer struct {
	runtime OCIRuntime
}

func NewExecStreamer(runtime OCIRuntime) *ExecStreamer {
	return &ExecStreamer{runtime: runtime}
}

func (s *ExecStreamer) StreamIn(log lager.Logger, handle string, spec garden.StreamInSpec) error {
	user := spec.User
	if user == "" {
		user = "root"
	}

	var mkdirStderr streamStderr
	if err := s.execAndWait(log, handle, garden.ProcessSpec{
		Path: "/bin/mkdir",
		Args: []string{"-p", "--", spec.Path},
		User: user,
	}, garden.ProcessIO{Stdout: io.Discard, Stderr: &mkdirStderr}); err != nil {
		return fmt.Errorf("stream-in: mkdir: %w: %s", err, mkdirStderr.String())
	}

	var stderrBuf streamStderr
	if err := s.execAndWait(log, handle, garden.ProcessSpec{
		Path: "/bin/tar",
		Args: []string{"-xf", "-", "-C", spec.Path},
		User: user,
	}, garden.ProcessIO{Stdin: spec.TarStream, Stdout: io.Discard, Stderr: &stderrBuf}); err != nil {
		return fmt.Errorf("stream-in: tar: %w: %s", err, stderrBuf.String())
	}

	return nil
}

func (s *ExecStreamer) StreamOut(log lager.Logger, handle string, spec garden.StreamOutSpec) (io.ReadCloser, error) {
	path := spec.Path
	sourcePath := filepath.Dir(path)
	compressPath := filepath.Base(path)
	if strings.HasSuffix(path, "/") {
		sourcePath = path
		compressPath = "."
	}

	reader, writer := io.Pipe()

	user := spec.User
	if user == "" {
		user = "root"
	}

	var stderrBuf streamStderr
	process, err := s.runtime.Exec(log, handle, garden.ProcessSpec{
		Path: "/bin/tar",
		Args: []string{"-cf", "-", "-C", sourcePath, "--", compressPath},
		User: user,
	}, garden.ProcessIO{Stdout: streamOutputWriter{writer}, Stderr: &stderrBuf})
	if err != nil {
		writer.Close()
		reader.Close()
		return nil, fmt.Errorf("stream-out: exec tar: %w", err)
	}

	done := make(chan struct{})
	go func() {
		defer close(done) // signals after writer closure, not just Wait
		exitCode, waitErr := process.Wait()
		if waitErr != nil {
			tarErr := fmt.Errorf("stream-out: tar wait: %w: %s", waitErr, stderrBuf.String())
			log.Error("stream-out-tar-failed", tarErr)
			writer.CloseWithError(tarErr)
		} else if exitCode != 0 {
			tarErr := fmt.Errorf("stream-out: tar exited %d: %s", exitCode, stderrBuf.String())
			log.Error("stream-out-tar-failed", tarErr)
			writer.CloseWithError(tarErr)
		} else {
			writer.Close()
		}
	}()

	return &execStreamReader{PipeReader: reader, process: process, done: done}, nil
}

type streamOutputWriter struct {
	writer *io.PipeWriter
}

func (writer streamOutputWriter) Write(data []byte) (int, error) {
	written, err := writer.writer.Write(data)
	if err == io.ErrClosedPipe {
		return len(data), nil
	}
	return written, err
}

type execStreamReader struct {
	*io.PipeReader
	process      garden.Process
	done         <-chan struct{} // fires after goroutine exits (writer closed)
	close        sync.Once
	closeErr     error
	signalFailed bool
}

func (s *execStreamReader) Close() error {
	s.close.Do(func() {
		s.closeErr = s.PipeReader.Close()
		select {
		case <-s.done:
			return
		default:
		}
		if err := s.process.Signal(garden.SignalKill); err != nil {
			// Signal failed. The process may have already exited (benign)
			// or may be genuinely unreachable. Do a bounded wait on done:
			// if the worker finishes promptly, the failure was benign.
			select {
			case <-s.done:
				// Worker finished — signal failure was benign (process
				// already exited). done closing means writer.Close()
				// also completed (defer ordering).
				return
			case <-time.After(time.Second):
				// Worker did not finish within 1s — genuine failure.
				s.closeErr = fmt.Errorf("stream-out: cancel tar: %w", err)
				s.signalFailed = true
			}
		}
	})
	// wait for the streamer worker to finish writer closure after
	// process.Wait, so Close proves full worker termination. Skip if the
	// kill signal genuinely failed against a live process.
	if !s.signalFailed {
		<-s.done
	}
	return s.closeErr
}

type streamStderr struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (s *streamStderr) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.Write(data)
}

func (s *streamStderr) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buffer.String()
}

func (s *ExecStreamer) execAndWait(log lager.Logger, handle string, spec garden.ProcessSpec, pio garden.ProcessIO) error {
	process, err := s.runtime.Exec(log, handle, spec, pio)
	if err != nil {
		return err
	}
	exitCode, err := process.Wait()
	if err != nil {
		return err
	}
	if exitCode != 0 {
		return fmt.Errorf("exited %d", exitCode)
	}
	return nil
}
