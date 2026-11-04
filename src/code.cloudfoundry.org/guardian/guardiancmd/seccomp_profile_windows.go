package guardiancmd

import "github.com/opencontainers/runtime-spec/specs-go"

// loadSeccompProfile is a no-op on Windows: buildSeccomp returns nil there and
// seccomp is a Linux-only container control.
func loadSeccompProfile(path string, fallback *specs.LinuxSeccomp) (*specs.LinuxSeccomp, error) {
	return fallback, nil
}
