package runcontainerd

import (
	"errors"
	"fmt"
	"io"
	"sync"

	"code.cloudfoundry.org/garden"
	"code.cloudfoundry.org/guardian/rundmc/goci"
	"code.cloudfoundry.org/lager/v3"
)

//counterfeiter:generate . PeaManager
type PeaManager interface {
	Create(log lager.Logger, id string, bundle goci.Bndl, io garden.ProcessIO) error
	Start(log lager.Logger, id string) error
	Delete(log lager.Logger, containerID string) error
	RemoveBundle(log lager.Logger, containerID string) error
}

//counterfeiter:generate . Volumizer
type Volumizer interface {
	Destroy(log lager.Logger, handle string) error
}

// PeaRuntimeCleanupError keeps a failed child's image available for a later
// cleanup retry when runtime deletion has not been proved.
type PeaRuntimeCleanupError struct{ Err error }

func (e *PeaRuntimeCleanupError) Error() string        { return e.Err.Error() }
func (e *PeaRuntimeCleanupError) Unwrap() error        { return e.Err }
func (e *PeaRuntimeCleanupError) PeaRootfsInUse() bool { return true }

type RunContainerPea struct {
	peaManager     PeaManager
	processManager ProcessManager
	volumizer      Volumizer
	mu             sync.Mutex
	processes      map[string]*peaAttachment
}

type peaAttachment struct {
	parent         string
	process        *PeaProcess
	stdout, stderr peaOutput
}

// TaskAttacher loads an existing child with one real task IO reader after a
// Guardian restart. It is deliberately separate from parent exec attachment.
type TaskAttacher interface {
	AttachTask(log lager.Logger, id string, pio garden.ProcessIO) (BackingProcess, error)
}

func NewRunContainerPea(peaManager PeaManager, processManager ProcessManager, volumizer Volumizer) *RunContainerPea {
	return &RunContainerPea{
		peaManager:     peaManager,
		processManager: processManager,
		volumizer:      volumizer,
		processes:      make(map[string]*peaAttachment),
	}
}

func (r *RunContainerPea) RunPea(
	log lager.Logger, processID string, processBundle goci.Bndl, sandboxHandle string,
	pio garden.ProcessIO, tty bool, procJSON io.Reader, extraCleanup func() error,
) (_ garden.Process, result error) {

	if processBundle.Spec.Annotations == nil {
		processBundle.Spec.Annotations = make(map[string]string)
	}
	processBundle.Spec.Annotations["container-type"] = "pea"
	processBundle.Spec.Annotations["sandbox-container"] = sandboxHandle

	log.Info("using runcontainerd.RunPea")
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := &peaAttachment{parent: sandboxHandle}
	entry.stdout.add(pio.Stdout)
	entry.stderr.add(pio.Stderr)
	pio.Stdout, pio.Stderr = &entry.stdout, &entry.stderr

	if err := r.peaManager.Create(log, processID, processBundle, pio); err != nil {
		return nil, err
	}
	// Create succeeded, so this invocation owns the child. Do not use this
	// rollback for a failed Create (e.g. an ID collision with an existing pea).
	defer func() {
		if result != nil {
			if err := r.peaManager.Delete(log, processID); err != nil {
				result = &PeaRuntimeCleanupError{Err: errors.Join(result, err)}
				return // retain metadata/rootfs while runtime cleanup is unproved
			}
			result = errors.Join(result, r.peaManager.RemoveBundle(log, processID))
		}
	}()

	process, err := r.processManager.GetTask(log, processID)
	if err != nil {
		return nil, err
	}
	// Unlike a parent, the pea already has its parent's network and needs no
	// intervening CNI stage. A Garden Run request must return a started process.
	if err := r.peaManager.Start(log, processID); err != nil {
		return nil, err
	}

	// PeaProcess owns task + metadata + image deletion; do not delete its task
	// once through Process.Wait and then again through the pea manager.
	r.register(log, processID, process, entry)
	return entry.process, nil
}

// register is called with r.mu held. Drop only this incarnation after successful
// cleanup; old attached handles retain their cached Wait result, not a new task.
func (r *RunContainerPea) register(log lager.Logger, id string, process BackingProcess, entry *peaAttachment) {
	entry.process = NewPeaProcess(log, *NewProcess(log, process, false), r.peaManager, r.volumizer)
	entry.process.onClean = func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.processes[id] == entry {
			delete(r.processes, id)
		}
	}
	r.processes[id] = entry
}

func (r *RunContainerPea) Attach(log lager.Logger, parent, id string, pio garden.ProcessIO) (garden.Process, error) {
	if pio.Stdin != nil {
		// Garden HTTP always supplies an input pipe, including when the client
		// attaches output only. Close that endpoint rather than rejecting the
		// attachment or forwarding bytes into startup-owned stdin. Close does
		// not read or wait for the client, and a later pipe write fails.
		input, transportPipe := pio.Stdin.(*io.PipeReader)
		if !transportPipe {
			return nil, fmt.Errorf("cannot attach stdin to an image-backed pea after Start")
		}
		if err := input.CloseWithError(fmt.Errorf("image-backed pea attachment stdin is closed")); err != nil {
			return nil, err
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	entry := r.processes[id]
	if entry != nil && entry.parent != parent {
		return nil, garden.ProcessNotFoundError{ProcessID: id}
	}
	if entry == nil {
		attacher, ok := r.processManager.(TaskAttacher)
		if !ok {
			return nil, fmt.Errorf("child task attachment is unavailable")
		}
		entry = &peaAttachment{parent: parent}
		entry.stdout.add(pio.Stdout)
		entry.stderr.add(pio.Stderr)
		process, err := attacher.AttachTask(log, id, garden.ProcessIO{Stdout: &entry.stdout, Stderr: &entry.stderr})
		if err != nil {
			return nil, err
		}
		r.register(log, id, process, entry)
	} else {
		entry.stdout.add(pio.Stdout)
		entry.stderr.add(pio.Stderr)
	}
	return entry.process, nil
}

// cleanMissingTask is used only after an owned pea's task was reported absent.
// Confirm absence via Delete before touching its image. No exit code or Wait is
// fabricated: this cleans an orphan whose original Wait result is unavailable.
func (r *RunContainerPea) cleanMissingTask(log lager.Logger, parent, id string) error {
	r.mu.Lock()
	if entry := r.processes[id]; entry != nil {
		r.mu.Unlock()
		if entry.parent != parent {
			return garden.ProcessNotFoundError{ProcessID: id}
		}
		_, err := entry.process.Wait()
		return err
	}
	defer r.mu.Unlock()
	if err := r.peaManager.Delete(log, id); err != nil {
		return err
	}
	if err := r.volumizer.Destroy(log, id); err != nil {
		return err
	}
	return r.peaManager.RemoveBundle(log, id)
}
