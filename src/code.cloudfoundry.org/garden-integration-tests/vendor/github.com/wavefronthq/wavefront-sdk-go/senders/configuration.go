package senders

import (
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wavefronthq/wavefront-sdk-go/internal/auth"
)

const (
	defaultCSPBaseURL    = "https://console.cloud.vmware.com/"
	defaultTracesPort    = 30001
	defaultMetricsPort   = 2878
	defaultBatchSize     = 10_000
	defaultBufferSize    = 50_000
	defaultFlushInterval = 1 * time.Second
	defaultTimeout       = 10 * time.Second
)

// Configuration for the direct ingestion sender
type configuration struct {
	Server string // Wavefront URL of the form https://<INSTANCE>.wavefront.com

	// Optional configuration properties. Default values should suffice for most use cases.
	// override the defaults only if you wish to set higher values.

	MetricsPort int
	TracesPort  int

	// max batch of data sent per flush interval. defaults to 10,000. recommended not to exceed 40,000.
	BatchSize int

	// send, or don't send, internal SDK metrics that begin with ~sdk.go.core
	SendInternalMetrics bool

	// size of internal buffers beyond which received data is dropped.
	// helps with handling brief increases in data and buffering on errors.
	// separate buffers are maintained per data type (metrics, spans and distributions)
	// buffers are not pre-allocated to max size and vary based on actual usage.
	// defaults to 500,000. higher values could use more memory.
	MaxBufferSize int

	// interval (in seconds) at which to flush data to Wavefront. defaults to 1 Second.
	// together with batch size controls the max theoretical throughput of the sender.
	FlushInterval           time.Duration
	SDKMetricsTags          map[string]string
	Path                    string
	Authentication          interface{}
	httpClientConfiguration *httpClientConfiguration
	HTTPClient              *http.Client
}

func (c *configuration) Direct() bool {
	return c.Authentication != nil
}

func createConfig(wfURL string, setters ...Option) (*configuration, error) {
	cfg := &configuration{
		MetricsPort:             defaultMetricsPort,
		TracesPort:              defaultTracesPort,
		BatchSize:               defaultBatchSize,
		MaxBufferSize:           defaultBufferSize,
		FlushInterval:           defaultFlushInterval,
		SendInternalMetrics:     true,
		SDKMetricsTags:          map[string]string{},
		httpClientConfiguration: &httpClientConfiguration{Timeout: defaultTimeout},
	}

	u, err := url.Parse(wfURL)
	if err != nil {
		return nil, err
	}

	if len(u.User.String()) > 0 {
		cfg.Authentication = auth.APIToken{
			Token: u.User.String(),
		}
		u.User = nil
	}

	for _, set := range setters {
		set(cfg)
	}

	switch strings.ToLower(u.Scheme) {
	case "http":
		if cfg.Direct() {
			log.Println("Detecting wavefront direct ingestion, will attempt to connect port 80.")
			cfg.setDefaultPort(80)
		}
	case "https":
		if cfg.Direct() {
			log.Println("Detecting wavefront direct ingestion, will attempt to connect port 443.")
			cfg.setDefaultPort(443)
		}
	default:
		return nil, fmt.Errorf("invalid scheme '%s' in '%s', only 'http/https' is supported", u.Scheme, u)
	}

	if u.Path != "" {
		cfg.Path = u.Path
		u.Path = ""
	}

	if u.Port() != "" {
		port, err := strconv.Atoi(u.Port())
		if err != nil {
			return nil, fmt.Errorf("unable to convert port to integer: %s", err)
		}
		cfg.setDefaultPort(port)
		u.Host = u.Hostname()
	}
	cfg.Server = u.String()

	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{
			Timeout: cfg.httpClientConfiguration.Timeout,
			Transport: &http.Transport{
				TLSClientConfig: cfg.httpClientConfiguration.TLSClientConfig,
			},
		}
	}

	return cfg, nil
}

func (c *configuration) setDefaultPort(port int) {
	c.MetricsPort = port
	c.TracesPort = port
}

func (c *configuration) tracesURL() string {
	return fmt.Sprintf("%s:%d%s", c.Server, c.TracesPort, c.Path)
}

func (c *configuration) metricsURL() string {
	return fmt.Sprintf("%s:%d%s", c.Server, c.MetricsPort, c.Path)
}

func (c *configuration) MetricPrefix() string {
	result := "~sdk.go.core.sender.proxy"
	if c.Direct() {
		result = "~sdk.go.core.sender.direct"
	}
	return result
}
