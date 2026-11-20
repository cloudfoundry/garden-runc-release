package runcontainerd

import (
	"fmt"
	"sync"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/lager/v3"
)

type PeaProcess struct {
	Process
	peaManager     PeaManager
	volumizer      Volumizer
	waitOnce       sync.Once
	exitCode       int
	waitErr        error
	stateMu        sync.Mutex
	exited         bool
	cleanupMu      sync.Mutex
	taskDeleted    bool
	imageDestroyed bool
	cleaned        bool
	onClean        func()
}

func NewPeaProcess(log lager.Logger, process Process, peaManager PeaManager, volumizer Volumizer) *PeaProcess {
	return &PeaProcess{
		Process:    process,
		peaManager: peaManager,
		volumizer:  volumizer,
	}
}

func (p *PeaProcess) Wait() (int, error) {
	p.waitOnce.Do(func() {
		p.exitCode, p.waitErr = p.Process.Wait()
		if p.waitErr == nil {
			p.stateMu.Lock()
			p.exited = true
			p.stateMu.Unlock()
		}
	})
	if p.waitErr != nil {
		return 0, p.waitErr
	}
	// Run, Attach and restart cleanup share this process. Only one caller may
	// remove its task/image; a failed cleanup may be retried without re-Waiting.
	p.cleanupMu.Lock()
	defer p.cleanupMu.Unlock()
	if p.cleaned {
		return p.exitCode, nil
	}

	if !p.taskDeleted {
		if err := p.peaManager.Delete(p.log, p.ID()); err != nil {
			return 0, err
		}
		p.taskDeleted = true
	}
	// Keep the ownership metadata while image deletion is fallible, so a
	// restarted cleaner can discover an orphan and an ID cannot be reused.
	if !p.imageDestroyed {
		if err := p.volumizer.Destroy(p.log, p.ID()); err != nil {
			return 0, err
		}
		p.imageDestroyed = true
	}
	if err := p.peaManager.RemoveBundle(p.log, p.ID()); err != nil {
		return 0, err
	}

	p.cleaned = true
	if p.onClean != nil {
		p.onClean()
	}
	return p.exitCode, nil
}

func (p *PeaProcess) Signal(signal garden.Signal) error {
	p.stateMu.Lock()
	defer p.stateMu.Unlock()
	if p.exited {
		return fmt.Errorf("pea %s has exited", p.ID())
	}
	return p.Process.Signal(signal)
}
