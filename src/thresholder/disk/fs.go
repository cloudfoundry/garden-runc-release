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
		AvailableBlocks: int64(fsStat.Bavail),
		BlockSize:       fsStat.Bsize,
	}, nil
}
