package bundlerules

import (
	spec "code.cloudfoundry.org/guardian/gardener/container-spec"
	"code.cloudfoundry.org/guardian/rundmc/goci"
)

// CgroupNamespace gives unprivileged containers a cgroup namespace on cgroup v2
// so /proc/self/cgroup reports 0::/ instead of the host path. Privileged
// containers and cgroup v1 are left unchanged, matching containerd's CRI.
type CgroupNamespace struct {
	IsCgroup2UnifiedMode func() bool
}

func (c CgroupNamespace) Apply(bndl goci.Bndl, spec spec.DesiredContainerSpec) (goci.Bndl, error) {
	if spec.Privileged {
		return bndl, nil
	}

	if c.IsCgroup2UnifiedMode != nil && c.IsCgroup2UnifiedMode() {
		bndl = bndl.WithNamespace(goci.CgroupNamespace)
	}

	return bndl, nil
}
