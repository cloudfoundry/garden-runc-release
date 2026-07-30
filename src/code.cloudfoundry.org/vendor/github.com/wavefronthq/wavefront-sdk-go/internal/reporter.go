package internal

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"strings"

	"github.com/wavefronthq/wavefront-sdk-go/internal/auth"
)

// The implementation of a Reporter that reports points directly to a Wavefront server.
type reporter struct {
	serverURL    string
	tokenService auth.Service
	client       *http.Client
}

// NewReporter creates a metrics Reporter
func NewReporter(server string, tokenService auth.Service, client *http.Client) Reporter {
	return &reporter{
		serverURL:    server,
		tokenService: tokenService,
		client:       client,
	}
}

// Report creates and sends a POST to the reportEndpoint with the given pointLines
func (reporter reporter) Report(format string, pointLines string) (*http.Response, error) {
	if format == "" || pointLines == "" {
		return nil, formatError
	}

	requestBody, err := linesToGzippedBytes(pointLines)
	if err != nil {
		return &http.Response{}, err
	}

	req, err := reporter.buildRequest(format, requestBody)
	if err != nil {
		return &http.Response{}, err
	}

	return reporter.execute(req)
}

func linesToGzippedBytes(pointLines string) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	_, err := zw.Write([]byte(pointLines))
	if err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err = zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), err
}

func (reporter reporter) buildRequest(format string, body []byte) (*http.Request, error) {
	apiURL := reporter.serverURL + reportEndpoint
	req, err := http.NewRequest("POST", apiURL, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set(contentType, octetStream)
	req.Header.Set(contentEncoding, gzipFormat)

	err = reporter.tokenService.Authorize(req)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add(formatKey, format)
	req.URL.RawQuery = q.Encode()
	return req, nil
}

func (reporter reporter) ReportEvent(event string) (*http.Response, error) {
	if event == "" {
		return nil, formatError
	}

	apiURL := reporter.serverURL + eventEndpoint
	req, err := http.NewRequest("POST", apiURL, strings.NewReader(event))
	if err != nil {
		return &http.Response{}, err
	}

	req.Header.Set(contentType, applicationJSON)

	if reporter.IsDirect() {
		req.Header.Set(contentEncoding, gzipFormat)
	}

	err = reporter.tokenService.Authorize(req)
	if err != nil {
		return nil, err
	}

	return reporter.execute(req)
}

func (reporter reporter) execute(req *http.Request) (*http.Response, error) {
	resp, err := reporter.client.Do(req)
	if err != nil {
		return resp, err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	defer resp.Body.Close()
	return resp, nil
}

func (reporter reporter) Close() {
	reporter.tokenService.Close()
}

func (reporter reporter) IsDirect() bool {
	return reporter.tokenService.IsDirect()
}
