package senders

import (
	"fmt"

	"github.com/wavefronthq/wavefront-sdk-go/internal"
	"github.com/wavefronthq/wavefront-sdk-go/internal/sdkmetrics"
)

// NewSender creates a Sender using the provided URL and Options
func NewSender(wfURL string, setters ...Option) (Sender, error) {
	cfg, err := createConfig(wfURL, setters...)
	if err != nil {
		return nil, fmt.Errorf("unable to create sender config: %s", err)
	}

	tokenService := tokenServiceForCfg(cfg)
	client := cfg.HTTPClient
	metricsReporter := internal.NewReporter(cfg.metricsURL(), tokenService, client)
	tracesReporter := internal.NewReporter(cfg.tracesURL(), tokenService, client)

	sender := &realSender{
		defaultSource: internal.GetHostname("wavefront_direct_sender"),
		proxy:         !cfg.Direct(),
	}
	if cfg.SendInternalMetrics {
		sender.internalRegistry = sender.realInternalRegistry(cfg)
	} else {
		sender.internalRegistry = sdkmetrics.NewNoOpRegistry()
	}
	sender.pointHandler = newLineHandler(metricsReporter, cfg, internal.MetricFormat, "points", sender.internalRegistry)
	sender.histoHandler = newLineHandler(metricsReporter, cfg, internal.HistogramFormat, "histograms", sender.internalRegistry)
	sender.spanHandler = newLineHandler(tracesReporter, cfg, internal.TraceFormat, "spans", sender.internalRegistry)
	sender.spanLogHandler = newLineHandler(tracesReporter, cfg, internal.SpanLogsFormat, "span_logs", sender.internalRegistry)
	sender.eventHandler = newLineHandler(metricsReporter, cfg, internal.EventFormat, "events", sender.internalRegistry)

	sender.Start()
	return sender, nil
}

func copyTags(orig map[string]string) map[string]string {
	result := make(map[string]string, len(orig))
	for key, value := range orig {
		result[key] = value
	}
	return result
}
