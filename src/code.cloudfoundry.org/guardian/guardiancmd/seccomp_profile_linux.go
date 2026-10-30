package guardiancmd

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/opencontainers/runtime-spec/specs-go"
)

// loadSeccompProfile reads a full OCI seccomp profile (the JSON representation
// of runtime-spec linux.seccomp) from path and returns it in place of the
// compiled-in baseline.
//
// The profile is applied as-is: its contents are the operator's responsibility
// (see the garden.seccomp_profile property description). loadSeccompProfile does
// not validate the policy -- it does not check the defaultAction, the rule set,
// or whether the result is more or less permissive than the compiled-in default.
//
// Fail-safe semantics:
//   - An empty path returns the compiled-in fallback unchanged (no override).
//   - A missing/unreadable/invalid-JSON file is a hard error: guardian must not
//     silently fall back to a weaker-or-different policy than the operator
//     intended, and must never start a container with an unintended profile.
func loadSeccompProfile(path string, fallback *specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	if path == "" {
		return fallback, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading seccomp profile %q: %w", path, err)
	}

	profile := &specs.LinuxSeccomp{}
	if err := json.Unmarshal(data, profile); err != nil {
		return nil, fmt.Errorf("parsing seccomp profile %q: %w", path, err)
	}

	return profile, nil
}
