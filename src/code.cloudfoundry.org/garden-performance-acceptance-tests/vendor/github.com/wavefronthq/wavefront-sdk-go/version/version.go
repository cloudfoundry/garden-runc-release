package version

import (
	"runtime/debug"
	"strings"
)

const (
	unavailable = "unavailable"
)

var Version = getVersion()

func getVersion() string {
	return scanForVersion(debug.ReadBuildInfo())
}

func scanForVersion(bi *debug.BuildInfo, ok bool) string {
	if !ok {
		return unavailable
	}
	for _, deps := range bi.Deps {
		if deps.Path == "github.com/wavefronthq/wavefront-sdk-go" {
			return strings.TrimPrefix(deps.Version, "v")
		}
	}
	return unavailable
}
