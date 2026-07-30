package commandrunner

import (
	"context"
	"errors"
	"os/exec"
)

type CommandRunner struct {
}

func (c CommandRunner) Run(ctx context.Context, command string, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, command, args...).Output()

	if ctx.Err() == context.DeadlineExceeded {
		return output, ctx.Err()
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		return output, errors.New(string(exitErr.Stderr))
	}

	return output, nil
}
