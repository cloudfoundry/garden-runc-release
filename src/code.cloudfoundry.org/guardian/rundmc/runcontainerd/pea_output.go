package runcontainerd

import (
	"io"
	"sync"
)

// peaOutput is the single FIFO reader's destination. Attach adds a destination,
// never another FIFO reader. Writers have the usual ProcessIO backpressure
// contract; a disconnected writer is removed without poisoning other clients.
type peaOutput struct {
	mu      sync.Mutex
	writers []io.Writer
}

func (o *peaOutput) add(w io.Writer) {
	if w == nil {
		return
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.writers = append(o.writers, w)
}

func (o *peaOutput) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	kept := o.writers[:0]
	for _, w := range o.writers {
		if n, err := w.Write(b); err == nil && n == len(b) {
			kept = append(kept, w)
		}
	}
	// Do not retain references to disconnected clients in the backing array.
	clear(o.writers[len(kept):])
	o.writers = kept
	return len(b), nil
}
