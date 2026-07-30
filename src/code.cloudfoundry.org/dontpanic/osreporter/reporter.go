package osreporter

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/logrusorgru/aurora"
)

type Reporter struct {
	stdout     io.Writer
	reportPath string
	collectors []RegisteredCollector
}

//go:generate counterfeiter . Collector
type Collector interface {
	Run(context.Context, string, io.Writer) error
}

func New(reportPath string, stdout io.Writer) Reporter {
	return Reporter{
		reportPath: reportPath,
		stdout:     stdout,
	}
}

func (r *Reporter) RegisterCollector(name string, collector Collector, timeout ...time.Duration) {
	r.registerCollector(name, collector, false, timeout...)
}

func (r *Reporter) RegisterNoisyCollector(name string, collector Collector, timeout ...time.Duration) {
	r.registerCollector(name, collector, true, timeout...)
}

func (r *Reporter) registerCollector(name string, collector Collector, echoOutput bool, timeout ...time.Duration) {
	maxDuration := 10 * time.Second
	if len(timeout) > 0 {
		maxDuration = timeout[0]
	}

	registeredCollector := RegisteredCollector{
		collector:  collector,
		name:       name,
		timeout:    maxDuration,
		echoOutput: echoOutput,
	}
	r.collectors = append(r.collectors, registeredCollector)
}

func (r Reporter) Run() error {
	fmt.Fprintln(r.stdout, aurora.Green("<Useful information below, please copy-paste from here>").Bold())

	logFile, err := os.Create(filepath.Join(r.reportPath, "dontpanic.log"))
	if err != nil {
		return err
	}

	for _, collector := range r.collectors {
		r.logHeader(logFile, collector.name)

		out := io.Discard
		if collector.echoOutput {
			out = r.stdout
		}

		err := collector.Run(r.reportPath, out)
		if err != nil {
			r.logError(logFile, collector.name, err)
		}
	}

	if err := r.createTarball(); err != nil {
		return err
	}

	fmt.Fprintln(r.stdout, aurora.Green(fmt.Sprintf("<Report Complete. Archive Created: %s.tar.gz>", r.reportPath)).Bold())

	return os.RemoveAll(r.reportPath)
}

func (r Reporter) logHeader(writer io.Writer, value string) {
	header := "## " + value
	fmt.Fprintln(r.stdout, aurora.Cyan(header).Bold())
	fmt.Fprintln(writer, header)
}

func (r Reporter) logError(writer io.Writer, subject string, err error) {
	errorMessage := fmt.Sprintf(">> %s failed: %s", subject, err.Error())
	fmt.Fprintln(r.stdout, aurora.Red(errorMessage))
	fmt.Fprintln(writer, errorMessage)
}

func (r Reporter) createTarball() error {
	return exec.Command("tar", "czf", r.reportPath+".tar.gz", "-C", filepath.Dir(r.reportPath), filepath.Base(r.reportPath)).Run()
}

type RegisteredCollector struct {
	collector  Collector
	name       string
	echoOutput bool
	timeout    time.Duration
}

func (p RegisteredCollector) Run(dstPath string, out io.Writer) error {
	ctx, cancel := context.WithTimeout(context.Background(), p.timeout)
	defer cancel()

	err := p.collector.Run(ctx, dstPath, out)
	if err == context.DeadlineExceeded {
		return fmt.Errorf("timed out after %s", p.timeout)
	}

	return err
}
