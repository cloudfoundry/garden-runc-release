package sdkmetrics

// mimics senders.MetricSender to avoid circular dependency
type internalSender interface {
	// SendMetric sends a single metric to Wavefront with optional timestamp and tags.
	SendMetric(name string, value float64, ts int64, source string, tags map[string]string) error

	// SendDeltaCounter sends a delta counter (counter aggregated at the Wavefront service) to Wavefront.
	// the timestamp for a delta counter is assigned at the server side.
	SendDeltaCounter(name string, value float64, source string, tags map[string]string) error
}

type Incrementer interface {
	Inc()
}

type Registry interface {
	Start()
	Stop()

	PointsTracker() SuccessTracker
	HistogramsTracker() SuccessTracker
	SpansTracker() SuccessTracker
	SpanLogsTracker() SuccessTracker
	EventsTracker() SuccessTracker

	NewGauge(s string, f func() int64) *FunctionalGauge
	Flush()
}
