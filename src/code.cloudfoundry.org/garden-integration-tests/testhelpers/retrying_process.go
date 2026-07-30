package testhelpers

import (
	"code.cloudfoundry.org/garden"
)

const MAX_WAIT_RETRIES = 5

type RetryingProcess struct {
	Process garden.Process
}

func (p *RetryingProcess) ID() string {
	return p.Process.ID()
}

func (p *RetryingProcess) Wait() (int, error) {
	var err error
	for i := 0; i < MAX_WAIT_RETRIES; i++ {
		var exitCode int
		exitCode, err = p.Process.Wait()
		if err == nil {
			return exitCode, nil
		}
	}
	return -1, err

}
func (p *RetryingProcess) SetTTY(ttySpec garden.TTYSpec) error {
	return p.Process.SetTTY(ttySpec)
}
func (p *RetryingProcess) Signal(signal garden.Signal) error {
	return p.Process.Signal(signal)
}
