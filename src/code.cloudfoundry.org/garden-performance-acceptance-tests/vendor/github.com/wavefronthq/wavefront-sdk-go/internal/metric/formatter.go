package metric

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/wavefronthq/wavefront-sdk-go/internal"
)

// Gets a metric line in the Wavefront metrics data format:
// <metricName> <metricValue> [<timestamp>] source=<source> [pointTags]
// Example: "new-york.power.usage 42422.0 1533531013 source=localhost datacenter=dc1"
func Line(name string, value float64, ts int64, source string, tags map[string]string, defaultSource string) (string, error) {
	if name == "" {
		return "", errors.New("empty metric name")
	}

	if source == "" {
		source = defaultSource
	}

	sb := internal.GetBuffer()
	defer internal.PutBuffer(sb)

	sb.WriteString(strconv.Quote(internal.Sanitize(name)))
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatFloat(value, 'f', -1, 64))

	if ts != 0 {
		sb.WriteString(" ")
		sb.WriteString(strconv.FormatInt(ts, 10))
	}

	sb.WriteString(" source=")
	sb.WriteString(internal.SanitizeValue(source))

	for k, v := range tags {
		if v == "" {
			return "", fmt.Errorf("tag values cannot be empty: metric=%s tag=%s", name, k)
		}
		sb.WriteString(" ")
		sb.WriteString(strconv.Quote(internal.Sanitize(k)))
		sb.WriteString("=")
		sb.WriteString(internal.SanitizeValue(v))
	}
	sb.WriteString("\n")
	return sb.String(), nil
}
