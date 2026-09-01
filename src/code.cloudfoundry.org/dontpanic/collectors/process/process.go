package process

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"context"

	"code.cloudfoundry.org/dontpanic/commandrunner"
)

type Collector struct {
	destinationPath string
	runner          commandrunner.CommandRunner
}

func NewCollector(destinationPath string) Collector {
	return Collector{
		destinationPath: destinationPath,
		runner:          commandrunner.CommandRunner{},
	}
}

func (c Collector) Run(ctx context.Context, reportDir string, stdout io.Writer) error {
	procs, err := c.runner.Run(ctx, "sh", "-c", "ps -eLo tid | tail -n +2")
	if err != nil {
		return err
	}

	for _, procLine := range strings.Split(string(procs), "\n") {
		proc := strings.Trim(string(procLine), " ")

		procDir := filepath.Join(reportDir, c.destinationPath, proc)
		if err := os.MkdirAll(procDir, 0755); err != nil {
			return err
		}

		if err := c.collectProcData(ctx, fmt.Sprintf("ls -lah /proc/%s/fd", proc), filepath.Join(procDir, "fd")); err != nil {
			continue
		}

		if err := c.collectProcData(ctx, fmt.Sprintf("ls -lah /proc/%s/ns", proc), filepath.Join(procDir, "ns")); err != nil {
			continue
		}

		if err := c.collectProcData(ctx, fmt.Sprintf("cat /proc/%s/cgroup", proc), filepath.Join(procDir, "cgroup")); err != nil {
			continue
		}

		if err := c.collectProcData(ctx, fmt.Sprintf("cat /proc/%s/status", proc), filepath.Join(procDir, "status")); err != nil {
			continue
		}

		if err := c.collectProcData(ctx, fmt.Sprintf("cat /proc/%s/stack", proc), filepath.Join(procDir, "stack")); err != nil {
			continue
		}
	}

	return nil
}

func (c *Collector) collectProcData(ctx context.Context, command, destFile string) error {
	out, err := c.runner.Run(ctx, "sh", "-c", command)
	if err != nil {
		return err
	}
	err = os.WriteFile(destFile, out, 0644)
	if err != nil {
		return err
	}

	return nil
}
