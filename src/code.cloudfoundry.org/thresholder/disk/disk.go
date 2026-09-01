package disk

import (
	"fmt"
)

type Stat struct {
	AvailableBlocks int64
	BlockSize       int64
}

//go:generate counterfeiter . FS

type FS interface {
	Stat(path string) (Stat, error)
}

type Meter struct {
	fs FS
}

func NewMeter() Meter {
	return NewMeterWithFS(NewSysFS())
}

func NewMeterWithFS(fs FS) Meter {
	return Meter{fs: fs}
}

func (d Meter) GetAvailableSpace(path string) (int64, error) {
	stat, err := d.fs.Stat(path)
	if err != nil {
		return 0, fmt.Errorf("cannot stat %s: %w", path, err)
	}

	return stat.BlockSize * stat.AvailableBlocks, nil
}
