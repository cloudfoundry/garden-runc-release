package nerd_test

import (
	"errors"
	"testing"
)

type deleteSpecSetup struct {
	createAttempted bool
}

func (setup *deleteSpecSetup) create(create func() error) error {
	setup.createAttempted = true
	return create()
}

func (setup *deleteSpecSetup) cleanup(cleanup func()) {
	if setup.createAttempted {
		cleanup()
	}
}

func TestDeleteSpecSetupOwnership(t *testing.T) {
	createFailure := errors.New("partial Create failure")
	for _, test := range []struct {
		name    string
		attempt bool
		failure error
	}{
		{name: "skipped before Nerd initialization"},
		{name: "successful Create", attempt: true},
		{name: "failed Create", attempt: true, failure: createFailure},
	} {
		t.Run(test.name, func(t *testing.T) {
			var setup deleteSpecSetup
			if test.attempt {
				if err := setup.create(func() error {
					if !setup.createAttempted {
						t.Fatal("Create attempt must own cleanup before it executes")
					}
					return test.failure
				}); err != test.failure {
					t.Fatalf("Create error = %v, want %v", err, test.failure)
				}
			}
			cleanupCalls := 0
			setup.cleanup(func() { cleanupCalls++ })
			if cleanupCalls != 0 && !test.attempt || cleanupCalls != 1 && test.attempt {
				t.Fatalf("cleanup calls = %d, Create attempted = %v", cleanupCalls, test.attempt)
			}
			setup = deleteSpecSetup{}
			setup.cleanup(func() { t.Fatal("skipped spec used stale setup from the previous spec") })
		})
	}
}

func TestDeleteSpecSetupCleanupFailureIsNotHidden(t *testing.T) {
	for _, createFailure := range []error{nil, errors.New("partial Create failure")} {
		t.Run("Create error="+errorText(createFailure), func(t *testing.T) {
			var setup deleteSpecSetup
			setup.create(func() error { return createFailure })
			cleanupFailure := errors.New("unexpected State/Delete/RemoveBundle failure")
			defer func() {
				if recovered := recover(); recovered != cleanupFailure {
					t.Fatalf("cleanup failure = %v, want %v", recovered, cleanupFailure)
				}
			}()
			setup.cleanup(func() { panic(cleanupFailure) })
		})
	}
}

func errorText(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}
