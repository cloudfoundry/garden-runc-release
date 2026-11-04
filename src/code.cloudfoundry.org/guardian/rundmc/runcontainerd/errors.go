package runcontainerd

import "fmt"

type ContainerNotFoundError struct {
	Handle string
}

func (ce ContainerNotFoundError) Error() string {
	return fmt.Sprintf("container %s not found", ce.Handle)
}

// IsNotFound marks this error as a definitive resource-absence indicator.
func (ce ContainerNotFoundError) IsNotFound() bool { return true }

type TaskNotFoundError struct {
	Handle string
}

func (te TaskNotFoundError) Error() string {
	return fmt.Sprintf("task for container %s not found", te.Handle)
}

// IsNotFound marks this error as a definitive resource-absence indicator.
func (te TaskNotFoundError) IsNotFound() bool { return true }

type ProcessNotFoundError struct {
	Handle string
	ID     string
}

func (pe ProcessNotFoundError) Error() string {
	return fmt.Sprintf("process %s not found in container %s", pe.ID, pe.Handle)
}
