package peas_test

import (
	"errors"
	"testing"
	"time"

	"code.cloudfoundry.org/guardian/rundmc/peas"
	"code.cloudfoundry.org/guardian/rundmc/peas/peasfakes"
	"code.cloudfoundry.org/lager/v3"
	"code.cloudfoundry.org/lager/v3/lagertest"
)

type retainedPeas struct {
	handles []string
	err     error
	queries int
	done    chan string
}

func (r *retainedPeas) PeaHandles() ([]string, error) {
	r.queries++
	return r.handles, r.err
}

func (r *retainedPeas) WaitAndCleanPea(_ lager.Logger, id string) error {
	r.done <- id
	return nil
}

func TestPeaCleanerDiscoversChildrenWithoutParentMetadata(t *testing.T) {
	runtime := new(peasfakes.FakeRuntime)
	// No parent metadata exists. The historical parent-first scan finds nothing.
	imageBacked := &retainedPeas{handles: []string{"retained-child", "live-child"}, done: make(chan string, 2)}
	done := imageBacked.done
	cleaner := &peas.PeaCleaner{Runtime: runtime, ImageBackedPeas: imageBacked}
	if err := cleaner.CleanAll(lagertest.NewTestLogger("retained-peas")); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range imageBacked.handles {
		select {
		case id := <-done:
			if seen[id] {
				t.Fatalf("duplicate cleanup %q", id)
			}
			seen[id] = true
		case <-time.After(100 * time.Millisecond):
			t.Fatal("parentless pea was not passed to shared task cleanup")
		}
	}
	if !seen["retained-child"] || !seen["live-child"] || imageBacked.queries != 1 || runtime.ContainerHandlesCallCount() != 0 || runtime.ContainerPeaHandlesCallCount() != 0 {
		t.Fatalf("wrong discovery: seen=%v queries=%d parent=%d child=%d", seen, imageBacked.queries, runtime.ContainerHandlesCallCount(), runtime.ContainerPeaHandlesCallCount())
	}
}

func TestPeaCleanerPropagatesDirectDiscoveryFailure(t *testing.T) {
	want := errors.New("pea metadata unavailable")
	imageBacked := &retainedPeas{handles: []string{"unsafe-partial-result"}, err: want, done: make(chan string, 1)}
	cleaner := &peas.PeaCleaner{Runtime: new(peasfakes.FakeRuntime), ImageBackedPeas: imageBacked}
	if err := cleaner.CleanAll(lagertest.NewTestLogger("retained-peas")); !errors.Is(err, want) {
		t.Fatalf("lost discovery error: %v", err)
	}
	select {
	case id := <-imageBacked.done:
		t.Fatalf("cleaned partial discovery %s", id)
	default:
	}
}
