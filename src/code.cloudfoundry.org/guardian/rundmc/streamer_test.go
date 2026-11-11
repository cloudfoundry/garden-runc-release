package rundmc_test

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"sync"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/garden/gardenfakes"
	"code.cloudfoundry.org/guardian/rundmc"
	fakes "code.cloudfoundry.org/guardian/rundmc/rundmcfakes"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ExecStreamer", func() {
	var (
		fakeRuntime *fakes.FakeOCIRuntime
		streamer    *rundmc.ExecStreamer
		logger      = lagertest.NewTestLogger("exec-streamer-test")
	)

	BeforeEach(func() {
		fakeRuntime = new(fakes.FakeOCIRuntime)
		streamer = rundmc.NewExecStreamer(fakeRuntime)
	})

	Describe("StreamIn", func() {
		var newFakeProcess func(exitCode int, waitErr error) *gardenfakes.FakeProcess

		BeforeEach(func() {
			newFakeProcess = func(exitCode int, waitErr error) *gardenfakes.FakeProcess {
				p := new(gardenfakes.FakeProcess)
				p.WaitReturns(exitCode, waitErr)
				return p
			}
		})

		It("execs mkdir then tar with the supplied user and path", func() {
			mkdirProc := newFakeProcess(0, nil)
			tarProc := newFakeProcess(0, nil)
			fakeRuntime.ExecReturnsOnCall(0, mkdirProc, nil)
			fakeRuntime.ExecReturnsOnCall(1, tarProc, nil)

			tarContent := bytes.NewBufferString("tar-payload")
			err := streamer.StreamIn(logger, "some-handle", garden.StreamInSpec{
				Path:      "/dst/dir",
				User:      "alice",
				TarStream: tarContent,
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(fakeRuntime.ExecCallCount()).To(Equal(2))

			// First call: mkdir
			_, handle, spec, pio := fakeRuntime.ExecArgsForCall(0)
			Expect(handle).To(Equal("some-handle"))
			Expect(spec.Path).To(Equal("/bin/mkdir"))
			Expect(spec.Args).To(Equal([]string{"-p", "--", "/dst/dir"}))
			Expect(spec.User).To(Equal("alice"))
			Expect(pio.Stdout).NotTo(BeNil())
			Expect(pio.Stderr).NotTo(BeNil())

			// Second call: tar
			_, handle, spec, pio = fakeRuntime.ExecArgsForCall(1)
			Expect(handle).To(Equal("some-handle"))
			Expect(spec.Path).To(Equal("/bin/tar"))
			Expect(spec.Args).To(Equal([]string{"-xf", "-", "-C", "/dst/dir"}))
			Expect(spec.User).To(Equal("alice"))
			Expect(pio.Stdin).To(Equal(tarContent))
		})

		It("defaults user to root when empty", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(0, nil), nil)
			fakeRuntime.ExecReturnsOnCall(1, newFakeProcess(0, nil), nil)

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				User:      "",
				TarStream: strings.NewReader(""),
			})
			Expect(err).NotTo(HaveOccurred())

			_, _, spec, _ := fakeRuntime.ExecArgsForCall(0)
			Expect(spec.User).To(Equal("root"))
			_, _, spec, _ = fakeRuntime.ExecArgsForCall(1)
			Expect(spec.User).To(Equal("root"))
		})

		It("returns an error when mkdir exec fails", func() {
			fakeRuntime.ExecReturnsOnCall(0, nil, errors.New("exec failed"))

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: mkdir")))
			Expect(err).To(MatchError(ContainSubstring("exec failed")))
			Expect(fakeRuntime.ExecCallCount()).To(Equal(1)) // tar not attempted
		})

		It("returns an error when mkdir exits nonzero", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(1, nil), nil)

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: mkdir")))
			Expect(err).To(MatchError(ContainSubstring("exited 1")))
			Expect(fakeRuntime.ExecCallCount()).To(Equal(1))
		})

		It("returns an error when mkdir Wait fails", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(0, errors.New("wait err")), nil)

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: mkdir")))
			Expect(err).To(MatchError(ContainSubstring("wait err")))
		})

		It("returns an error when tar exec fails", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(0, nil), nil)
			fakeRuntime.ExecReturnsOnCall(1, nil, errors.New("tar exec failed"))

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: tar")))
			Expect(err).To(MatchError(ContainSubstring("tar exec failed")))
		})

		It("returns an error when tar exits nonzero", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(0, nil), nil)
			fakeRuntime.ExecReturnsOnCall(1, newFakeProcess(2, nil), nil)

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: tar")))
			Expect(err).To(MatchError(ContainSubstring("exited 2")))
		})

		It("returns an error when tar Wait fails", func() {
			fakeRuntime.ExecReturnsOnCall(0, newFakeProcess(0, nil), nil)
			fakeRuntime.ExecReturnsOnCall(1, newFakeProcess(0, errors.New("tar wait")), nil)

			err := streamer.StreamIn(logger, "h", garden.StreamInSpec{
				Path:      "/p",
				TarStream: strings.NewReader(""),
			})
			Expect(err).To(MatchError(ContainSubstring("stream-in: tar")))
			Expect(err).To(MatchError(ContainSubstring("tar wait")))
		})
	})

	Describe("StreamOut", func() {
		It("execs tar with the correct path splitting for a file path", func() {
			tarProc := new(gardenfakes.FakeProcess)
			tarProc.WaitReturns(0, nil)
			fakeRuntime.ExecReturns(tarProc, nil)

			reader, err := streamer.StreamOut(logger, "some-handle", garden.StreamOutSpec{
				Path: "/some/dir/file.txt",
				User: "bob",
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(reader).NotTo(BeNil())
			reader.Close()

			Expect(fakeRuntime.ExecCallCount()).To(Equal(1))
			_, handle, spec, pio := fakeRuntime.ExecArgsForCall(0)
			Expect(handle).To(Equal("some-handle"))
			Expect(spec.Path).To(Equal("/bin/tar"))
			Expect(spec.Args).To(Equal([]string{"-cf", "-", "-C", "/some/dir", "--", "file.txt"}))
			Expect(spec.User).To(Equal("bob"))
			Expect(pio.Stdout).NotTo(BeNil()) // connected to pipe writer
		})

		It("uses sourcePath=path and compressPath=. for trailing-slash paths", func() {
			tarProc := new(gardenfakes.FakeProcess)
			tarProc.WaitReturns(0, nil)
			fakeRuntime.ExecReturns(tarProc, nil)

			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
				Path: "/some/dir/",
			})
			Expect(err).NotTo(HaveOccurred())
			reader.Close()

			_, _, spec, _ := fakeRuntime.ExecArgsForCall(0)
			Expect(spec.Args).To(Equal([]string{"-cf", "-", "-C", "/some/dir/", "--", "."}))
		})

		It("defaults user to root when empty", func() {
			tarProc := new(gardenfakes.FakeProcess)
			tarProc.WaitReturns(0, nil)
			fakeRuntime.ExecReturns(tarProc, nil)

			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
				Path: "/p",
				User: "",
			})
			Expect(err).NotTo(HaveOccurred())
			reader.Close()

			_, _, spec, _ := fakeRuntime.ExecArgsForCall(0)
			Expect(spec.User).To(Equal("root"))
		})

		It("returns an error when exec fails and cleans up pipes", func() {
			fakeRuntime.ExecReturns(nil, errors.New("exec boom"))

			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
				Path: "/p",
			})
			Expect(err).To(MatchError(ContainSubstring("stream-out: exec tar")))
			Expect(err).To(MatchError(ContainSubstring("exec boom")))
			Expect(reader).To(BeNil())
		})

		It("delivers tar output through the returned reader on success", func() {
			payload := []byte("tar-archive-bytes")
			tarProc := new(gardenfakes.FakeProcess)

			var writerFromPIO io.Writer
			var mu sync.Mutex
			fakeRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {
				mu.Lock()
				writerFromPIO = pio.Stdout
				mu.Unlock()
				return tarProc, nil
			}

			// Wait blocks until signalled, then returns success
			waitDone := make(chan struct{})
			tarProc.WaitStub = func() (int, error) {
				<-waitDone
				return 0, nil
			}

			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
				Path: "/p",
			})
			Expect(err).NotTo(HaveOccurred())

			// io.Pipe is synchronous: write blocks until read consumes.
			// Run the simulated tar write in a goroutine so the main
			// goroutine can read concurrently.
			writeErr := make(chan error, 1)
			go func() {
				mu.Lock()
				w := writerFromPIO
				mu.Unlock()
				_, err := w.Write(payload)
				writeErr <- err
				close(waitDone) // let Wait return → writer.Close → EOF
			}()

			data, readErr := io.ReadAll(reader)
			Expect(readErr).NotTo(HaveOccurred())
			Expect(data).To(Equal(payload))
			Expect(<-writeErr).NotTo(HaveOccurred())
		})

		Context("late-error propagation", func() {
			It("propagates a nonzero tar exit code to the reader", func() {
				tarProc := new(gardenfakes.FakeProcess)
				started := make(chan struct{})
				fakeRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, _ garden.ProcessIO) (garden.Process, error) {
					close(started)
					return tarProc, nil
				}
				tarProc.WaitReturns(2, nil)

				reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
					Path: "/p",
				})
				Expect(err).NotTo(HaveOccurred())
				<-started

				_, readErr := io.ReadAll(reader)
				Expect(readErr).To(HaveOccurred())
				Expect(readErr.Error()).To(ContainSubstring("exit"))
			})

			It("propagates a Wait error to the reader", func() {
				tarProc := new(gardenfakes.FakeProcess)
				started := make(chan struct{})
				fakeRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, _ garden.ProcessIO) (garden.Process, error) {
					close(started)
					return tarProc, nil
				}
				tarProc.WaitReturns(0, errors.New("wait failed"))

				reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
					Path: "/p",
				})
				Expect(err).NotTo(HaveOccurred())
				<-started

				_, readErr := io.ReadAll(reader)
				Expect(readErr).To(HaveOccurred())
				Expect(readErr.Error()).To(ContainSubstring("wait failed"))
			})

			It("returns clean EOF on success", func() {
				tarProc := new(gardenfakes.FakeProcess)
				tarProc.WaitReturns(0, nil)
				fakeRuntime.ExecReturns(tarProc, nil)

				reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
					Path: "/p",
				})
				Expect(err).NotTo(HaveOccurred())

				data, readErr := io.ReadAll(reader)
				Expect(readErr).NotTo(HaveOccurred())
				Expect(data).To(BeEmpty())
			})
		})

		It("discards writes after the reader closes", func() {
			tarProc := new(gardenfakes.FakeProcess)
			waitDone := make(chan struct{})
			tarProc.SignalStub = func(signal garden.Signal) error {
				Expect(signal).To(Equal(garden.SignalKill))
				close(waitDone)
				return nil
			}
			tarProc.WaitStub = func() (int, error) {
				<-waitDone
				return 0, nil
			}

			var writerFromPIO io.Writer
			fakeRuntime.ExecStub = func(_ lager.Logger, _ string, _ garden.ProcessSpec, pio garden.ProcessIO) (garden.Process, error) {
				writerFromPIO = pio.Stdout
				return tarProc, nil
			}

			reader, err := streamer.StreamOut(logger, "h", garden.StreamOutSpec{
				Path: "/p",
			})
			Expect(err).NotTo(HaveOccurred())

			Expect(reader.Close()).To(Succeed())
			written, writeErr := writerFromPIO.Write([]byte("data"))
			Expect(writeErr).NotTo(HaveOccurred())
			Expect(written).To(Equal(len("data")))
		})
	})
})
