package nerd

import (
	"io"
	"testing"
)

type lastRead struct{}

func (lastRead) Read(b []byte) (int, error) { return copy(b, "last bytes"), io.EOF }

func TestPeaInputCompletionFollowsLastWrite(t *testing.T) {
	done := make(chan struct{})
	p := &peaInput{reader: lastRead{}, done: done}
	b := make([]byte, 32)
	n, err := p.Read(b)
	if n != 10 || err != nil {
		t.Fatalf("last bytes = %d, %v", n, err)
	}
	select {
	case <-done:
		t.Fatal("completion announced before last write")
	default:
	}
	if n, err := p.Read(b); n != 0 || err != io.EOF {
		t.Fatalf("EOF = %d, %v", n, err)
	}
	select {
	case <-done:
	default:
		t.Fatal("completion not announced")
	}
	if _, err := p.Read(b); err != io.EOF {
		t.Fatal(err)
	}
}
