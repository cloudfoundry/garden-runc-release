package bundlerules

import (
	spec "code.cloudfoundry.org/guardian/gardener/container-spec"
	"code.cloudfoundry.org/guardian/rundmc/goci"
)

// CgroupNamespace gives containers a cgroup namespace on cgroup v2 cells.
// Without it /proc/self/cgroup leaks the host-absolute cgroup path and the
// cgroup2 mount is rooted at the host hierarchy; the namespace pins the view so
// /proc/self/cgroup reports 0::/ and the mount is scoped to the container's own
// subtree. Applied to privileged and unprivileged containers alike so both get
// the same view. Gated to cgroup v2 so v1 (hybrid) cells are unaffected.
type CgroupNamespace struct {
	IsCgroup2UnifiedMode func() bool
}

func (c CgroupNamespace) Apply(bndl goci.Bndl, spec spec.DesiredContainerSpec) (goci.Bndl, error) {
	if c.IsCgroup2UnifiedMode != nil && c.IsCgroup2UnifiedMode() {
		bndl = bndl.WithNamespace(goci.CgroupNamespace)
	}

	return bndl, nil
}
