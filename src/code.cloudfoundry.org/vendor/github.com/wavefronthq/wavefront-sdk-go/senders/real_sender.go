package senders

import (
	"fmt"
	"os"
	"strconv"

	"github.com/wavefronthq/wavefront-sdk-go/event"
	"github.com/wavefronthq/wavefront-sdk-go/histogram"
	"github.com/wavefronthq/wavefront-sdk-go/internal"
	eventInternal "github.com/wavefronthq/wavefront-sdk-go/internal/event"
	histogramInternal "github.com/wavefronthq/wavefront-sdk-go/internal/histogram"
	"github.com/wavefronthq/wavefront-sdk-go/internal/metric"
	"github.com/wavefronthq/wavefront-sdk-go/internal/sdkmetrics"
	"github.com/wavefronthq/wavefront-sdk-go/internal/span"
	"github.com/wavefronthq/wavefront-sdk-go/version"
)

// Sender Interface for sending metrics, distributions and spans to Wavefront
type Sender interface {
	MetricSender
	DistributionSender
	SpanSender
	EventSender
	internal.Flusher
	Close()
	private()
}

type realSender struct {
	reporter         internal.Reporter
	defaultSource    string
	pointHandler     internal.LineHandler
	histoHandler     internal.LineHandler
	spanHandler      internal.LineHandler
	spanLogHandler   internal.LineHandler
	eventHandler     internal.LineHandler
	internalRegistry sdkmetrics.Registry
	proxy            bool
}

func newLineHandler(reporter internal.Reporter, cfg *configuration, format, prefix string, registry sdkmetrics.Registry) *internal.RealLineHandler {
	opts := []internal.LineHandlerOption{internal.SetHandlerPrefix(prefix), internal.SetRegistry(registry)}
	batchSize := cfg.BatchSize
	if format == internal.EventFormat {
		batchSize = 1
		opts = append(opts, internal.SetLockOnThrottledError(true))
	}

	return internal.NewLineHandler(reporter, format, cfg.FlushInterval, batchSize, cfg.MaxBufferSize, opts...)
}

func (sender *realSender) Start() {
	sender.pointHandler.Start()
	sender.histoHandler.Start()
	sender.spanHandler.Start()
	sender.spanLogHandler.Start()
	sender.internalRegistry.Start()
	sender.eventHandler.Start()
}

func (sender *realSender) private() {
}

func (sender *realSender) SendMetric(name string, value float64, ts int64, source string, tags map[string]string) error {
	line, err := metric.Line(name, value, ts, source, tags, sender.defaultSource)
	return trySendWith(
		line,
		err,
		sender.pointHandler,
		sender.internalRegistry.PointsTracker(),
	)
}

func (sender *realSender) SendDeltaCounter(name string, value float64, source string, tags map[string]string) error {
	if name == "" {
		sender.internalRegistry.PointsTracker().IncInvalid()
		return fmt.Errorf("empty metric name")
	}
	if !internal.HasDeltaPrefix(name) {
		name = internal.DeltaCounterName(name)
	}
	if value > 0 {
		return sender.SendMetric(name, value, 0, source, tags)
	}
	return nil
}

func (sender *realSender) SendDistribution(
	name string,
	centroids []histogram.Centroid,
	hgs map[histogram.Granularity]bool,
	ts int64,
	source string,
	tags map[string]string,
) error {
	line, err := histogramInternal.Line(name, centroids, hgs, ts, source, tags, sender.defaultSource)
	return trySendWith(
		line,
		err,
		sender.histoHandler,
		sender.internalRegistry.HistogramsTracker(),
	)
}

func trySendWith(line string, err error, handler internal.LineHandler, tracker sdkmetrics.SuccessTracker) error {
	if err != nil {
		tracker.IncInvalid()
		return err
	}

	tracker.IncValid()
	err = handler.HandleLine(line)
	if err != nil {
		tracker.IncDropped()
	}
	return err
}

func (sender *realSender) SendSpan(
	name string,
	startMillis, durationMillis int64,
	source, traceID, spanID string,
	parents, followsFrom []string,
	tags []SpanTag,
	spanLogs []SpanLog,
) error {

	logs := makeSpanLogs(spanLogs)
	line, err := span.Line(
		name,
		startMillis,
		durationMillis,
		source,
		traceID,
		spanID,
		parents,
		followsFrom,
		makeSpanTags(tags),
		logs,
		sender.defaultSource,
	)
	err = trySendWith(
		line,
		err,
		sender.spanHandler,
		sender.internalRegistry.SpansTracker())
	if err != nil {
		return err
	}

	if len(spanLogs) > 0 {
		logJSON, logJSONErr := span.LogJSON(traceID, spanID, logs, line)
		return trySendWith(
			logJSON,
			logJSONErr,
			sender.spanLogHandler,
			sender.internalRegistry.SpanLogsTracker())
	}
	return nil
}

func makeSpanTags(tags []SpanTag) []span.Tag {
	spanTags := make([]span.Tag, len(tags))
	for i, tag := range tags {
		spanTags[i] = span.Tag(tag)
	}
	return spanTags
}

func makeSpanLogs(logs []SpanLog) []span.Log {
	spanLogs := make([]span.Log, len(logs))
	for i, log := range logs {
		spanLogs[i] = span.Log(log)
	}
	return spanLogs
}

func (sender *realSender) SendEvent(
	name string,
	startMillis, endMillis int64,
	source string,
	tags map[string]string,
	setters ...event.Option,
) error {
	var line string
	var err error
	if sender.proxy {
		line, err = eventInternal.Line(name, startMillis, endMillis, source, tags, setters...)
	} else {
		line, err = eventInternal.LineJSON(name, startMillis, endMillis, source, tags, setters...)
	}

	return trySendWith(
		line,
		err,
		sender.eventHandler,
		sender.internalRegistry.EventsTracker(),
	)
}

func (sender *realSender) Close() {
	sender.pointHandler.Stop()
	sender.histoHandler.Stop()
	sender.spanHandler.Stop()
	sender.spanLogHandler.Stop()
	sender.internalRegistry.Stop()
	sender.eventHandler.Stop()
}

func (sender *realSender) Flush() error {
	errStr := ""
	err := sender.pointHandler.Flush()
	if err != nil {
		errStr = errStr + err.Error() + "\n"
	}
	err = sender.histoHandler.Flush()
	if err != nil {
		errStr = errStr + err.Error() + "\n"
	}
	err = sender.spanHandler.Flush()
	if err != nil {
		errStr = errStr + err.Error()
	}
	err = sender.spanLogHandler.Flush()
	if err != nil {
		errStr = errStr + err.Error()
	}
	err = sender.eventHandler.Flush()
	if err != nil {
		errStr = errStr + err.Error()
	}
	if errStr != "" {
		return fmt.Errorf(errStr)
	}
	return nil
}

func (sender *realSender) GetFailureCount() int64 {
	return sender.pointHandler.GetFailureCount() +
		sender.histoHandler.GetFailureCount() +
		sender.spanHandler.GetFailureCount() +
		sender.spanLogHandler.GetFailureCount() +
		sender.eventHandler.GetFailureCount()
}

func (sender *realSender) realInternalRegistry(cfg *configuration) sdkmetrics.Registry {
	var setters []sdkmetrics.RegistryOption

	setters = append(setters, sdkmetrics.SetPrefix(cfg.MetricPrefix()))
	setters = append(setters, sdkmetrics.SetTag("pid", strconv.Itoa(os.Getpid())))
	setters = append(setters, sdkmetrics.SetTag("version", version.Version))

	for key, value := range cfg.SDKMetricsTags {
		setters = append(setters, sdkmetrics.SetTag(key, value))
	}

	return sdkmetrics.NewMetricRegistry(
		sender,
		setters...,
	)
}
