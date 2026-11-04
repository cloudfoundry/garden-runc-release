package runcontainerd_test

import (
	"strings"
	"testing"

	"code.cloudfoundry.org/guardian/rundmc/runcontainerd"
)

// Unit tests for RuntimePolicy resolution. These verify the
// single source-of-truth mapping from containerd runtime type strings to
// the validated policy that governs all downstream decisions.
func TestResolveRuntimePolicy(t *testing.T) {
	tests := []struct {
		name        string
		runtimeType string
		wantPolicy  runcontainerd.RuntimePolicy
		wantErr     bool
	}{
		{
			name:        "empty string defaults to stock runc",
			runtimeType: "",
			wantPolicy:  runcontainerd.StockRuntime,
		},
		{
			name:        "explicit stock runc-v2 resolves to stock",
			runtimeType: runcontainerd.StockRuntimeType,
			wantPolicy:  runcontainerd.StockRuntime,
		},
		{
			name:        "runsc-v1 resolves to runsc policy",
			runtimeType: runcontainerd.SupportedRuntimeType,
			wantPolicy:  runcontainerd.RunscRuntime,
		},
		{
			name:        "unknown runtime is rejected",
			runtimeType: "io.containerd.kata.v2",
			wantErr:     true,
		},
		{
			name:        "similar but misspelled runtime is rejected",
			runtimeType: "io.containerd.runsc.v2",
			wantErr:     true,
		},
		{
			name:        "arbitrary string is rejected",
			runtimeType: "custom-runtime",
			wantErr:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy, err := runcontainerd.ResolveRuntimePolicy(tt.runtimeType)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("ResolveRuntimePolicy(%q) = %v, nil; want error", tt.runtimeType, policy)
				}
				// Error message should mention the unsupported type and both supported alternatives
				for _, want := range []string{tt.runtimeType, runcontainerd.StockRuntimeType, runcontainerd.SupportedRuntimeType} {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not mention %q", err.Error(), want)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveRuntimePolicy(%q) error: %v", tt.runtimeType, err)
			}
			if policy != tt.wantPolicy {
				t.Errorf("ResolveRuntimePolicy(%q) = %v, want %v", tt.runtimeType, policy, tt.wantPolicy)
			}
		})
	}
}

func TestRuntimePolicyIsAlternative(t *testing.T) {
	if runcontainerd.StockRuntime.IsAlternative() {
		t.Error("StockRuntime.IsAlternative() = true, want false")
	}
	if !runcontainerd.RunscRuntime.IsAlternative() {
		t.Error("RunscRuntime.IsAlternative() = false, want true")
	}
}

func TestRuntimePolicyString(t *testing.T) {
	tests := []struct {
		policy runcontainerd.RuntimePolicy
		want   string
	}{
		{runcontainerd.StockRuntime, "stock"},
		{runcontainerd.RunscRuntime, "runsc"},
	}
	for _, tt := range tests {
		if got := tt.policy.String(); got != tt.want {
			t.Errorf("%v.String() = %q, want %q", tt.policy, got, tt.want)
		}
	}
}

// TestResolveRuntimePolicyStockIdentity verifies that empty and explicit
// runc-v2 resolve to the exact same policy value. This is the acceptance
// criterion that "empty/explicit stock configurations select identical
// stock strategies".
func TestResolveRuntimePolicyStockIdentity(t *testing.T) {
	empty, err := runcontainerd.ResolveRuntimePolicy("")
	if err != nil {
		t.Fatal(err)
	}
	explicit, err := runcontainerd.ResolveRuntimePolicy(runcontainerd.StockRuntimeType)
	if err != nil {
		t.Fatal(err)
	}
	if empty != explicit {
		t.Errorf("empty policy %v != explicit runc policy %v; stock configurations must be identical", empty, explicit)
	}
}
