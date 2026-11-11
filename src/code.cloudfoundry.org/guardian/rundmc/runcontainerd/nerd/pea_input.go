package nerd

import "io"

// peaInput reports completion only on the read after the last bytes. cio's
// io.Copy has then written that preceding buffer to the FIFO. A reader may
// return data and EOF together; announcing EOF in that call would race the write.
type peaInput struct {
	reader io.Reader
	done   chan struct{}
	err    error
	closed bool
}

func (p *peaInput) Read(b []byte) (int, error) {
	if p.err != nil {
		if !p.closed {
			close(p.done)
			p.closed = true
		}
		return 0, p.err
	}
	n, err := p.reader.Read(b)
	if err != nil {
		p.err = err
		if n == 0 {
			return p.Read(b)
		}
		return n, nil
	}
	return n, nil
}
