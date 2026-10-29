package cgroups

import (
	cgroupslib "github.com/opencontainers/cgroups"
)

// removeCgroupDir removes a cgroup directory and all its sub-cgroups using
// recursive bottom-up rmdir. This is the correct way to remove cgroupv2
// directories because they contain kernel-managed virtual files (e.g.
// cgroup.procs, misc.events) that cannot be individually unlinked.
// os.RemoveAll fails with EPERM on these virtual files; unix.Rmdir on
// the directory itself removes them atomically.
//
// The vendored opencontainers/cgroups.RemovePath handles:
//   - recursive removal of child cgroup directories (bottom-up)
//   - EBUSY retry with exponential backoff for exiting processes
//   - ENOENT tolerance for already-removed paths
func removeCgroupDir(path string) error {
	return cgroupslib.RemovePath(path)
}
