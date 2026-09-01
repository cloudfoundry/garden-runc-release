package command

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"code.cloudfoundry.org/dontpanic/commandrunner"
)

type createOutputStream func(dir, filepath string) (io.WriteCloser, error)

type Collector struct {
	cmd                 string
	filename            string
	runner              commandrunner.CommandRunner
	outputStreamFactory createOutputStream
}

func NewCollector(cmd, filename string) Collector {
	return Collector{
		cmd:                 cmd,
		filename:            filename,
		runner:              commandrunner.CommandRunner{},
		outputStreamFactory: newFileOutputStream,
	}
}

func NewDiscardCollector(cmd string) Collector {
	return Collector{
		cmd:                 cmd,
		runner:              commandrunner.CommandRunner{},
		outputStreamFactory: newDiscardStream,
	}
}

func newDiscardStream(_, _ string) (io.WriteCloser, error) {
	return discardWriter{}, nil
}

func newFileOutputStream(destPath, filePath string) (io.WriteCloser, error) {
	outPath := filepath.Join(destPath, filePath)

	err := os.MkdirAll(filepath.Dir(outPath), 0755)
	if err != nil {
		return nil, err
	}

	return os.Create(outPath)
}

func (c Collector) Run(ctx context.Context, destPath string, stdout io.Writer) error {
	out, err := c.runner.Run(ctx, "sh", "-c", c.cmd)
	if err != nil {
		return err
	}

	outStream, err := c.outputStreamFactory(destPath, c.filename)
	if err != nil {
		return err
	}
	defer outStream.Close()

	_, err = io.MultiWriter(outStream, stdout).Write(out)
	if err != nil {
		return err
	}

	return nil
}

type discardWriter struct{}

func (dc discardWriter) Close() error {
	return nil
}

func (dc discardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
