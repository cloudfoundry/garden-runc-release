package file

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"strings"

	"code.cloudfoundry.org/dontpanic/commandrunner"
)

type Collector struct {
	sourcePath      string
	destinationPath string
	runner          commandrunner.CommandRunner
	archive         bool
}

func NewCollector(sourcePath, destinationPath string) Collector {
	return Collector{
		sourcePath:      sourcePath,
		destinationPath: destinationPath,
		runner:          commandrunner.CommandRunner{},
	}
}

func NewDirCollector(sourcePath, destinationPath string) Collector {
	return Collector{
		sourcePath:      sourcePath,
		destinationPath: destinationPath,
		runner:          commandrunner.CommandRunner{},
		archive:         true,
	}
}

func (c Collector) Run(ctx context.Context, reportDir string, stdout io.Writer) error {
	fullDestinationPath := filepath.Join(reportDir, c.destinationPath)
	toMake := fullDestinationPath
	if !strings.HasSuffix(c.destinationPath, "/") {
		toMake = filepath.Dir(toMake)
	}
	if err := os.MkdirAll(toMake, 0755); err != nil {
		return err
	}

	archive := ""
	if c.archive {
		archive = "-a"
	}
	cmd := fmt.Sprintf("cp %s %s %s", archive, c.sourcePath, fullDestinationPath)
	_, err := c.runner.Run(ctx, "sh", "-c", cmd)
	return err
}
