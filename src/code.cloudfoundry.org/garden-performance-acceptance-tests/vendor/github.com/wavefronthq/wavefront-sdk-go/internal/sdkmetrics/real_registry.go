package sdkmetrics

import (
	"sync"
	"time"
)

// realRegistry collects internal valid/invalid/dropped metrics and periodically sends them to Wavefront
type realRegistry struct {
	source       string
	prefix       string
	tags         map[string]string
	reportTicker *time.Ticker
	sender       internalSender
	done         chan struct{}

	mtx               sync.Mutex
	metrics           map[string]interface{}
	pointsTracker     *realSuccessTracker
	histogramsTracker *realSuccessTracker
	spansTracker      *realSuccessTracker
	spanLogsTracker   *realSuccessTracker
	eventsTracker     *realSuccessTracker
}

func (registry *realRegistry) Flush() {
	registry.report()
}

func (registry *realRegistry) PointsTracker() SuccessTracker {
	return registry.pointsTracker
}

func (registry *realRegistry) HistogramsTracker() SuccessTracker {
	return registry.histogramsTracker
}

func (registry *realRegistry) SpansTracker() SuccessTracker {
	return registry.spansTracker
}

func (registry *realRegistry) SpanLogsTracker() SuccessTracker {
	return registry.spanLogsTracker
}

func (registry *realRegistry) EventsTracker() SuccessTracker {
	return registry.eventsTracker
}

func (registry *realRegistry) newSuccessTracker(prefix string) *realSuccessTracker {
	return &realSuccessTracker{
		Valid:   registry.NewDeltaCounter(prefix + ".valid"),
		Invalid: registry.NewDeltaCounter(prefix + ".invalid"),
		Dropped: registry.NewDeltaCounter(prefix + ".dropped"),
	}
}

func NewMetricRegistry(sender internalSender, setters ...RegistryOption) Registry {
	registry := &realRegistry{
		sender:       sender,
		metrics:      make(map[string]interface{}),
		reportTicker: time.NewTicker(time.Second * 60),
		done:         make(chan struct{}),
	}

	registry.pointsTracker = registry.newSuccessTracker("points")
	registry.histogramsTracker = registry.newSuccessTracker("histograms")
	registry.spansTracker = registry.newSuccessTracker("spans")
	registry.spanLogsTracker = registry.newSuccessTracker("span_logs")
	registry.eventsTracker = registry.newSuccessTracker("events")

	for _, setter := range setters {
		setter(registry)
	}
	return registry
}

func (registry *realRegistry) Start() {
	go registry.start()
}

func (registry *realRegistry) start() {
	for {
		select {
		case <-registry.reportTicker.C:
			registry.report()
		case <-registry.done:
			return
		}
	}
}

func (registry *realRegistry) Stop() {
	registry.reportTicker.Stop()
	registry.done <- struct{}{}
}

// report sends internal SDK metrics and delta counters using an internalSender.
// We should not get any synchronous errors from the sender.Send Methods,
// and even if we did, there's not a good place to bubble them up to, so we ignore them.
func (registry *realRegistry) report() {
	registry.mtx.Lock()
	defer registry.mtx.Unlock()

	for k, metric := range registry.metrics {
		switch m := metric.(type) {
		case *DeltaCounter:
			deltaCount := m.count()
			_ = registry.sender.SendDeltaCounter(registry.prefix+"."+k, float64(deltaCount), "", registry.tags)
			metric.(*DeltaCounter).dec(deltaCount)
		case *MetricCounter:
			_ = registry.sender.SendMetric(registry.prefix+"."+k, float64(m.count()), 0, "", registry.tags)
		case *FunctionalGauge:
			_ = registry.sender.SendMetric(registry.prefix+"."+k, float64(m.instantValue()), 0, "", registry.tags)
		case *FunctionalGaugeFloat64:
			_ = registry.sender.SendMetric(registry.prefix+"."+k, m.instantValue(), 0, "", registry.tags)
		}
	}
}

func (registry *realRegistry) getOrAdd(name string, metric interface{}) interface{} {
	registry.mtx.Lock()
	defer registry.mtx.Unlock()

	if val, ok := registry.metrics[name]; ok {
		return val
	}
	registry.metrics[name] = metric
	return metric
}

func (registry *realRegistry) NewCounter(name string) *MetricCounter {
	return registry.getOrAdd(name, &MetricCounter{}).(*MetricCounter)
}

func (registry *realRegistry) NewDeltaCounter(name string) *DeltaCounter {
	return registry.getOrAdd(name, &DeltaCounter{MetricCounter{}}).(*DeltaCounter)
}

func (registry *realRegistry) NewGauge(name string, f func() int64) *FunctionalGauge {
	return registry.getOrAdd(name, &FunctionalGauge{value: f}).(*FunctionalGauge)
}

func (registry *realRegistry) NewGaugeFloat64(name string, f func() float64) *FunctionalGaugeFloat64 {
	return registry.getOrAdd(name, &FunctionalGaugeFloat64{value: f}).(*FunctionalGaugeFloat64)
}
