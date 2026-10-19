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
// The profile's defaultAction is validated at manifest-render time by the garden
// job template (an allowlist of terminal, non-permissive actions), so a bad
// override fails the deploy rather than a container start; loadSeccompProfile
// does not re-check it here.
//
// Fail-safe semantics:
//   - An empty path returns the compiled-in fallback unchanged (no override).
//   - A missing/unreadable/invalid file is a hard error: guardian must not
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
