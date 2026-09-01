package runcontainerd

import (
	"code.cloudfoundry.org/guardian/rundmc/goci"
)

func (m cgroupManager) setUnifiedResources(bundle goci.Bndl) error {
	return nil
}

func (m cgroupManager) addCgroupBindMount(bundle *goci.Bndl) {
	// no-op on Windows
}
