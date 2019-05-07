package calculator

func ShouldCollectGarbageOnCreate(reservedSpace int64) bool {
	return reservedSpace >= 0
}

func CalculateStoreSize(reservedSpace, diskSize, minStoreSize int64) int64 {
	storeSize := diskSize - positiveOrZero(reservedSpace)
	if storeSize > minStoreSize {
		return storeSize
	}

	return diskSize
}

func CalculateGCThreshold(reservedSpace, diskSize, minStoreSize int64) int64 {
	return positiveOrZero(diskSize - reservedSpace)
}

func positiveOrZero(n int64) int64 {
	if n < 0 {
		return 0
	}

	return n
}
