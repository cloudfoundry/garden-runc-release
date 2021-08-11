package disk

import (
	"fmt"
	"syscall"
)

type Stat struct {
	Blocks    int64
	BlockSize int64
}

type FS interface {
	Stat(path string) (Stat, error)
}

type SysFS struct{}

func NewSysFS() SysFS {
	return SysFS{}
}

func (fs SysFS) Stat(path string) (Stat, error) {
	var fsStat syscall.Statfs_t
	if err := syscall.Statfs(path, &fsStat); err != nil {
		return Stat{}, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	return Stat{
		Blocks:    int64(fsStat.Blocks),
		BlockSize: fsStat.Bsize,
	}, nil
}

type Meter struct {
	fs FS
}

func NewMeter() Meter {
	return Meter{fs: NewSysFS()}
}

func (d Meter) GetAvailableSpace(path string) (int64, error) {
	stat, err := d.fs.Stat(path)
	if err != nil {
		return 0, err
	}

	return stat.BlockSize * stat.Blocks, nil
}
