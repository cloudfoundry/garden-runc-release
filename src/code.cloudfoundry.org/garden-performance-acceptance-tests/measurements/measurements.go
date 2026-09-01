package measurements

import "errors"

type Measurements []float64

func (m Measurements) Average() (float64, error) {
	if len(m) == 0 {
		return 0.0, errors.New("no measurements recorded, cannot compute average")
	}

	sum := float64(0)

	for _, result := range m {
		sum += result
	}

	return sum / float64(len(m)), nil
}
