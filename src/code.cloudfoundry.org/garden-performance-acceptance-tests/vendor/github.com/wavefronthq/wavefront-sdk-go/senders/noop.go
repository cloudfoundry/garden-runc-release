package senders

import (
	"github.com/wavefronthq/wavefront-sdk-go/event"
	"github.com/wavefronthq/wavefront-sdk-go/histogram"
)

type noOpSender struct {
}

var (
	defaultNoopClient Sender = &noOpSender{}
)

// NewWavefrontNoOpClient returns a Wavefront Client instance for which all operations are no-ops.
func NewWavefrontNoOpClient() (Sender, error) {
	return defaultNoopClient, nil
}

func (sender *noOpSender) private() {
}

func (sender *noOpSender) Start() {
	// no-op
}

func (sender *noOpSender) SendMetric(string, float64, int64, string, map[string]string) error {
	return nil
}

func (sender *noOpSender) SendDeltaCounter(string, float64, string, map[string]string) error {
	return nil
}

func (sender *noOpSender) SendDistribution(string, []histogram.Centroid, map[histogram.Granularity]bool, int64, string, map[string]string) error {
	return nil
}

func (sender *noOpSender) SendSpan(string, int64, int64, string, string, string, []string, []string, []SpanTag, []SpanLog) error {
	return nil
}

func (sender *noOpSender) SendEvent(string, int64, int64, string, map[string]string, ...event.Option) error {
	return nil
}

func (sender *noOpSender) Close() {
	// no-op
}

func (sender *noOpSender) Flush() error {
	return nil
}

func (sender *noOpSender) GetFailureCount() int64 {
	return 0
}
