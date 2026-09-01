package auth

import "time"

func calculateNewTickerInterval(expiresIn time.Duration, fallback time.Duration) time.Duration {
	if expiresIn < (30 * time.Second) {
		return fallback
	}

	if expiresIn < (10 * time.Minute) {
		return expiresIn - (30 * time.Second)
	}
	return expiresIn - (3 * time.Minute)
}
