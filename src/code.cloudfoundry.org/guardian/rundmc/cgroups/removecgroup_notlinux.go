//go:build !linux

package cgroups

import "os"

// removeCgroupDir on non-Linux platforms falls back to os.RemoveAll.
// Cgroup virtual filesystems only exist on Linux; on other platforms
// (used for compilation checks and unit tests) os.RemoveAll is correct.
func removeCgroupDir(path string) error {
	return os.RemoveAll(path)
}
