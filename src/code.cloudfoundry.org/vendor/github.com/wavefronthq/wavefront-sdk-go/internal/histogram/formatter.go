package histogram

import (
	"bytes"
	"errors"
	"fmt"
	"strconv"

	"github.com/wavefronthq/wavefront-sdk-go/histogram"
	"github.com/wavefronthq/wavefront-sdk-go/internal"
)

// Line returns a histogram line in the Wavefront histogram data format:
// {!M | !H | !D} [<timestamp>] #<count> <mean> [centroids] <histogramName> source=<source> [pointTags]
// Example: "!M 1533531013 #20 30.0 #10 5.1 request.latency source=appServer1 region=us-west"
func Line(name string, centroids histogram.Centroids, hgs map[histogram.Granularity]bool, ts int64, source string, tags map[string]string, defaultSource string) (string, error) {
	if name == "" {
		return "", errors.New("empty distribution name")
	}

	if len(centroids) == 0 {
		return "", fmt.Errorf("distribution should have at least one centroid: histogram=%s", name)
	}

	if len(hgs) == 0 {
		return "", fmt.Errorf("histogram granularities cannot be empty: histogram=%s", name)
	}

	if source == "" {
		source = defaultSource
	}

	sb := internal.GetBuffer()
	defer internal.PutBuffer(sb)

	if ts != 0 {
		sb.WriteString(" ")
		sb.WriteString(strconv.FormatInt(ts, 10))
	}
	// Preprocess line. We know len(hgs) > 0 here.
	for _, centroid := range centroids.Compact() {
		sb.WriteString(" #")
		sb.WriteString(strconv.Itoa(centroid.Count))
		sb.WriteString(" ")
		sb.WriteString(strconv.FormatFloat(centroid.Value, 'f', -1, 64))
	}
	sb.WriteString(" ")
	sb.WriteString(strconv.Quote(internal.Sanitize(name)))
	sb.WriteString(" source=")
	sb.WriteString(internal.SanitizeValue(source))

	for k, v := range tags {
		if v == "" {
			return "", fmt.Errorf("tag values cannot be empty: histogram=%s tag=%s", name, k)
		}
		sb.WriteString(" ")
		sb.WriteString(strconv.Quote(internal.Sanitize(k)))
		sb.WriteString("=")
		sb.WriteString(internal.SanitizeValue(v))
	}
	sbBytes := sb.Bytes()

	sbg := bytes.Buffer{}
	for hg, on := range hgs {
		if on {
			sbg.WriteString(hg.String())
			sbg.Write(sbBytes)
			sbg.WriteString("\n")
		}
	}
	return sbg.String(), nil
}
