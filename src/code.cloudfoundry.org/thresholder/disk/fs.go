package disk

import (
	"syscall"
)

type SysFS struct{}

func NewSysFS() SysFS {
	return SysFS{}
}

func (fs SysFS) Stat(path string) (Stat, error) {
	var fsStat syscall.Statfs_t
	if err := syscall.Statfs(path, &fsStat); err != nil {
		return Stat{}, err
	}

	return Stat{
		// #nosec G115 - changing these attributes to uint64 has a bunch of knock on effects that would change grootfs interfaces. We are fine until filesystems are > 9.2 exabytes though
		AvailableBlocks: int64(fsStat.Bavail),
		BlockSize:       fsStat.Bsize,
	}, nil
}
