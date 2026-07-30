package calculator

import "math"

type Calculator interface {
	ShouldCollectGarbageOnCreate() bool
	CalculateStoreSize() int64
	CalculateGCThreshold() int64
}

type modernCalculator struct {
	reservedSpace     int64
	diskSize          int64
	minStoreSize      int64
	routineGC         bool
	gcThresholdFactor float64
}

// NewModernCalculator builds a Calculator that, by default, sets the GC
// threshold to (diskSize - reservedSpace). The threshold can be scaled down
// by providing a gcThresholdFactor in the range (0, 1]; values <= 0 or
// non-finite values fall back to 1.0 to preserve historical behaviour.
func NewModernCalculator(reservedSpace, diskSize, minStoreSize int64, routineGC bool, gcThresholdFactor float64) Calculator {
	if routineGC {
		reservedSpace = diskSize
	}

	if !isValidFactor(gcThresholdFactor) {
		gcThresholdFactor = 1.0
	}

	return &modernCalculator{
		reservedSpace:     reservedSpace,
		diskSize:          diskSize,
		minStoreSize:      minStoreSize,
		routineGC:         routineGC,
		gcThresholdFactor: gcThresholdFactor,
	}
}

func (m modernCalculator) ShouldCollectGarbageOnCreate() bool {
	return m.reservedSpace >= 0
}

func (m modernCalculator) CalculateStoreSize() int64 {
	storeSize := m.diskSize - positiveOrZero(m.reservedSpace)
	if storeSize > m.minStoreSize {
		return storeSize
	}

	return m.diskSize
}

func (m modernCalculator) CalculateGCThreshold() int64 {
	available := positiveOrZero(m.diskSize - positiveOrZero(m.reservedSpace))
	scaled := math.Floor(float64(available) * m.gcThresholdFactor)
	if scaled < 0 {
		return 0
	}
	return int64(scaled)
}

type oldFashionedCalculator struct {
	diskSize    int64
	gcThreshold int64
}

func NewOldFashionedCalculator(diskSize, gardenGcThreshold, grootfsGcThreshold int64) *oldFashionedCalculator {
	effectiveGcThreshold := grootfsGcThreshold
	if gardenGcThreshold > -1 {
		effectiveGcThreshold = gardenGcThreshold
	}

	return &oldFashionedCalculator{
		diskSize:    diskSize,
		gcThreshold: effectiveGcThreshold,
	}
}

func (o oldFashionedCalculator) ShouldCollectGarbageOnCreate() bool {
	return o.gcThreshold >= 0
}

func (o oldFashionedCalculator) CalculateStoreSize() int64 {
	return o.diskSize
}

func (o oldFashionedCalculator) CalculateGCThreshold() int64 {
	if o.gcThreshold > 0 {
		return o.gcThreshold
	}
	return o.diskSize + 1
}

func positiveOrZero(n int64) int64 {
	if n < 0 {
		return 0
	}

	return n
}

func isValidFactor(f float64) bool {
	if math.IsNaN(f) || math.IsInf(f, 0) {
		return false
	}
	return f > 0 && f <= 1
}
