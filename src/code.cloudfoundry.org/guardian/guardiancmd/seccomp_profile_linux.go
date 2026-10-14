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
// Fail-safe semantics:
//   - An empty path returns the compiled-in fallback unchanged (no override).
//   - A missing/unreadable/invalid file is a hard error: guardian must not
//     silently fall back to a weaker-or-different policy than the operator
//     intended, and must never start a container with an unintended profile.
//   - The profile must declare a non-permissive defaultAction. A default of
//     SCMP_ACT_ALLOW (default-permit) is rejected, so a bad override can never
//     widen the policy to allow-by-default.
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

	if err := validateSeccompProfile(profile); err != nil {
		return nil, fmt.Errorf("invalid seccomp profile %q: %w", path, err)
	}

	return profile, nil
}

func validateSeccompProfile(profile *specs.LinuxSeccomp) error {
	if profile.DefaultAction == "" {
		return fmt.Errorf("defaultAction must be set (e.g. SCMP_ACT_ERRNO); refusing a profile with no default action")
	}
	if profile.DefaultAction == specs.ActAllow {
		return fmt.Errorf("defaultAction SCMP_ACT_ALLOW is not permitted: it would make the policy default-permit and widen the container syscall surface")
	}
	return nil
}
