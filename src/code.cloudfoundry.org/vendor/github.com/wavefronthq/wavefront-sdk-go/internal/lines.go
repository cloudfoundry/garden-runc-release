package internal

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wavefronthq/wavefront-sdk-go/internal/auth"
	"github.com/wavefronthq/wavefront-sdk-go/internal/sdkmetrics"
)

const (
	MetricFormat    = "wavefront"
	HistogramFormat = "histogram"
	TraceFormat     = "trace"
	SpanLogsFormat  = "spanLogs"
	EventFormat     = "event"
)

type RealLineHandler struct {
	// keep these two fields as first element of struct
	// to guarantee 64-bit alignment on 32-bit machines.
	// atomic.* functions crash if operands are not 64-bit aligned.
	// See https://github.com/golang/go/issues/599
	failures  int64
	throttled int64

	Reporter      Reporter
	BatchSize     int
	MaxBufferSize int
	Format        string
	flushTicker   *time.Ticker

	internalRegistry sdkmetrics.Registry
	prefix           string

	mtx                sync.Mutex
	lockOnErrThrottled bool

	buffer chan string
	done   chan struct{}
}

var throttledSleepDuration = time.Second * 30
var errThrottled = errors.New("error: throttled event creation")

type LineHandlerOption func(*RealLineHandler)

func SetRegistry(registry sdkmetrics.Registry) LineHandlerOption {
	return func(handler *RealLineHandler) {
		handler.internalRegistry = registry
	}
}

func SetHandlerPrefix(prefix string) LineHandlerOption {
	return func(handler *RealLineHandler) {
		handler.prefix = prefix
	}
}

func SetLockOnThrottledError(lock bool) LineHandlerOption {
	return func(handler *RealLineHandler) {
		handler.lockOnErrThrottled = lock
	}
}

func NewLineHandler(reporter Reporter, format string, flushInterval time.Duration, batchSize, maxBufferSize int, setters ...LineHandlerOption) *RealLineHandler {
	lh := &RealLineHandler{
		Reporter:           reporter,
		BatchSize:          batchSize,
		MaxBufferSize:      maxBufferSize,
		flushTicker:        time.NewTicker(flushInterval),
		Format:             format,
		lockOnErrThrottled: false,
	}

	for _, setter := range setters {
		setter(lh)
	}

	if lh.internalRegistry != nil {
		lh.internalRegistry.NewGauge(lh.prefix+".queue.size", func() int64 {
			return int64(len(lh.buffer))
		})
		lh.internalRegistry.NewGauge(lh.prefix+".queue.remaining_capacity", func() int64 {
			return int64(lh.MaxBufferSize - len(lh.buffer))
		})
	}
	return lh
}

func (lh *RealLineHandler) Start() {
	lh.buffer = make(chan string, lh.MaxBufferSize)
	lh.done = make(chan struct{})

	go func() {
		for {
			select {
			case <-lh.flushTicker.C:
				err := lh.Flush()
				if err != nil {
					log.Println(lh.lockOnErrThrottled, "---", err)
					if err == errThrottled && lh.lockOnErrThrottled {
						go func() {
							lh.mtx.Lock()
							atomic.AddInt64(&lh.throttled, 1)
							log.Printf("sleeping for %v, buffer size: %d\n", throttledSleepDuration, len(lh.buffer))
							time.Sleep(throttledSleepDuration)
							lh.mtx.Unlock()
						}()
					}
				}
			case <-lh.done:
				return
			}
		}
	}()
}

func (lh *RealLineHandler) HandleLine(line string) error {
	select {
	case lh.buffer <- line:
		return nil
	default:
		atomic.AddInt64(&lh.failures, 1)
		return fmt.Errorf("buffer full, dropping line: %s", line)
	}
}

func minInt(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func (lh *RealLineHandler) Flush() error {
	lh.mtx.Lock()
	defer lh.mtx.Unlock()
	bufLen := len(lh.buffer)
	if bufLen > 0 {
		size := minInt(bufLen, lh.BatchSize)
		lines := make([]string, size)
		for i := 0; i < size; i++ {
			lines[i] = <-lh.buffer
		}
		return lh.report(lines)
	}
	return nil
}

func (lh *RealLineHandler) FlushAll() error {
	lh.mtx.Lock()
	defer lh.mtx.Unlock()
	bufLen := len(lh.buffer)
	if bufLen > 0 {
		var imod int
		size := minInt(bufLen, lh.BatchSize)
		lines := make([]string, size)
		for i := 0; i < bufLen; i++ {
			imod = i % size
			lines[imod] = <-lh.buffer
			if imod == size-1 { // report batch
				if err := lh.report(lines); err != nil {
					return err
				}
			}
		}
		if imod < size-1 { // report remaining
			return lh.report(lines[0 : imod+1])
		}
	}
	return nil
}

func (lh *RealLineHandler) report(lines []string) error {
	strLines := strings.Join(lines, "")
	var resp *http.Response
	var err error

	if lh.Format == EventFormat {
		resp, err = lh.Reporter.ReportEvent(strLines)
	} else {
		resp, err = lh.Reporter.Report(lh.Format, strLines)
	}

	if err != nil {
		if shouldRetry(err) {
			lh.bufferLines(lines)
		}
		return fmt.Errorf("error reporting %s format data to Wavefront: %q", lh.Format, err)
	}

	if 400 <= resp.StatusCode && resp.StatusCode <= 599 {
		atomic.AddInt64(&lh.failures, 1)
		lh.bufferLines(lines)
		if resp.StatusCode == 406 {
			return errThrottled
		}
		return fmt.Errorf("error reporting %s format data to Wavefront. status=%d", lh.Format, resp.StatusCode)
	}
	return nil
}

func shouldRetry(err error) bool {
	switch err.(type) {
	case *auth.Err:
		return false
	}
	return true
}

func (lh *RealLineHandler) bufferLines(batch []string) {
	log.Println("error reporting to Wavefront. buffering lines.")
	for _, line := range batch {
		_ = lh.HandleLine(line)
	}
}

func (lh *RealLineHandler) GetFailureCount() int64 {
	return atomic.LoadInt64(&lh.failures)
}

// GetThrottledCount returns the number of Throttled errors received.
func (lh *RealLineHandler) GetThrottledCount() int64 {
	return atomic.LoadInt64(&lh.throttled)
}

func (lh *RealLineHandler) Stop() {
	lh.flushTicker.Stop()
	lh.done <- struct{}{} // block until goroutine exits
	if err := lh.FlushAll(); err != nil {
		log.Println(err)
	}
	lh.done = nil
	lh.buffer = nil
}
