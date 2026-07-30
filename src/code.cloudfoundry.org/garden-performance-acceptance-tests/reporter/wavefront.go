package reporter

import (
	"errors"
	"fmt"
	"time"

	"code.cloudfoundry.org/lager/v3"
	"github.com/onsi/ginkgo/v2/config"
	"github.com/onsi/ginkgo/v2/types"
	wavefront "github.com/wavefronthq/wavefront-sdk-go/senders"
)

type WavefrontReporter struct {
	logger   lager.Logger
	hostName string
	wfSender wavefront.Sender
}

func NewWavefrontReporter(
	logger lager.Logger,
	hostName string,
	wfSender wavefront.Sender,
) WavefrontReporter {
	return WavefrontReporter{
		logger:   logger.Session("wavefront-reporter"),
		hostName: hostName,
		wfSender: wfSender,
	}
}

func (r *WavefrontReporter) SpecSuiteWillBegin(config config.GinkgoConfigType, summary *types.SuiteSummary) {
}

func (r *WavefrontReporter) BeforeSuiteDidRun(setupSummary *types.SetupSummary) {
}

func (r *WavefrontReporter) AfterSuiteDidRun(setupSummary *types.SetupSummary) {
}

func (r *WavefrontReporter) SpecWillRun(specSummary *types.SpecSummary) {
}

func (r *WavefrontReporter) SpecDidComplete(specSummary *types.SpecSummary) {
	if specSummary.Passed() && specSummary.IsMeasurement {
		for _, measurement := range specSummary.Measurements {
			if measurement.Info == nil {
				panic(fmt.Sprintf("%#v", specSummary))
			}

			info, ok := measurement.Info.(ReporterInfo)
			if !ok {
				r.logger.Error("failed-type-assertion-on-measurement-info", errors.New("type-assertion-failed"))
				continue
			}

			if info.MetricName == "" {
				r.logger.Error("failed-blank-metric-name", errors.New("blank-metric-name"))
				continue
			}

			timestamp := time.Now().Unix()
			r.logger.Info("sending-metrics-to-wavefront", lager.Data{"metric": info.MetricName, "hostName": r.hostName})
			if err := r.wfSender.SendMetric(fmt.Sprintf("%s-slowest", info.MetricName), measurement.Largest, timestamp, r.hostName, nil); err != nil {
				r.logger.Error("failed-sending-slowest-metrics-to-wavefront", err, lager.Data{"metric": info.MetricName, "hostName": r.hostName})
				continue
			}
			if err := r.wfSender.SendMetric(fmt.Sprintf("%s-fastest", info.MetricName), measurement.Smallest, timestamp, r.hostName, nil); err != nil {
				r.logger.Error("failed-sending-fastest-metrics-to-wavefront", err, lager.Data{"metric": info.MetricName, "hostName": r.hostName})
				continue
			}
			if err := r.wfSender.SendMetric(fmt.Sprintf("%s-average", info.MetricName), measurement.Average, timestamp, r.hostName, nil); err != nil {
				r.logger.Error("failed-sending-average-metrics-to-wavefront", err, lager.Data{"metric": info.MetricName, "hostName": r.hostName})
				continue
			}

			if err := r.wfSender.Flush(); err != nil {
				r.logger.Error("failed-flushing-to-wavefront", err, lager.Data{"metric": info.MetricName, "hostName": r.hostName})
				continue
			}

			r.logger.Info("sending-metrics-to-wavefront-complete", lager.Data{"metric": info.MetricName, "hostName": r.hostName})
		}
	}
}

func (r *WavefrontReporter) SpecSuiteDidEnd(summary *types.SuiteSummary) {
}
