package span

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/wavefronthq/wavefront-sdk-go/internal"
)

// Line gets a span line in the Wavefront span data format:
// <tracingSpanName> source=<source> [pointTags] <start_millis> <duration_milli_seconds>
// Example:
// "getAllUsers source=localhost traceId=7b3bf470-9456-11e8-9eb6-529269fb1459 spanId=0313bafe-9457-11e8-9eb6-529269fb1459
//
//	parent=2f64e538-9457-11e8-9eb6-529269fb1459 application=Wavefront http.method=GET 1533531013 343500"
func Line(name string, startMillis, durationMillis int64, source, traceID, spanID string, parents, followsFrom []string, tags []Tag, spanLogs []Log, defaultSource string) (string, error) {
	if name == "" {
		return "", errors.New("span name cannot be empty")
	}

	if source == "" {
		source = defaultSource
	}

	if !isUUIDFormat(traceID) {
		return "", fmt.Errorf("traceId is not in UUID format: span=%s traceId=%s", name, traceID)
	}
	if !isUUIDFormat(spanID) {
		return "", fmt.Errorf("spanId is not in UUID format: span=%s spanId=%s", name, spanID)
	}

	sb := internal.GetBuffer()
	defer internal.PutBuffer(sb)

	sb.WriteString(internal.SanitizeValue(name))
	sb.WriteString(" source=")
	sb.WriteString(internal.SanitizeValue(source))
	sb.WriteString(" traceId=")
	sb.WriteString(traceID)
	sb.WriteString(" spanId=")
	sb.WriteString(spanID)

	for _, parent := range parents {
		sb.WriteString(" parent=")
		sb.WriteString(parent)
	}

	for _, item := range followsFrom {
		sb.WriteString(" followsFrom=")
		sb.WriteString(item)
	}

	if len(spanLogs) > 0 {
		sb.WriteString(" ")
		sb.WriteString(strconv.Quote(internal.Sanitize("_spanLogs")))
		sb.WriteString("=")
		sb.WriteString(strconv.Quote(internal.Sanitize("true")))
	}

	for _, tag := range tags {
		if tag.Key == "" {
			return "", fmt.Errorf("tag keys cannot be empty: span=%s", name)
		}
		if tag.Value == "" {
			return "", fmt.Errorf("tag values cannot be empty: span=%s tag=%s", name, tag.Key)
		}
		sb.WriteString(" ")
		sb.WriteString(strconv.Quote(internal.Sanitize(tag.Key)))
		sb.WriteString("=")
		sb.WriteString(internal.SanitizeValue(tag.Value))
	}
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatInt(startMillis, 10))
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatInt(durationMillis, 10))
	sb.WriteString("\n")

	return sb.String(), nil
}

func LogJSON(traceID, spanID string, spanLogs []Log, span string) (string, error) {
	l := Logs{
		TraceID: traceID,
		SpanID:  spanID,
		Logs:    spanLogs,
		Span:    span,
	}
	out, err := json.Marshal(l)
	if err != nil {
		return "", err
	}
	return string(out[:]) + "\n", nil
}

func isUUIDFormat(str string) bool {
	l := len(str)
	if l != 36 {
		return false
	}
	for i := 0; i < l; i++ {
		c := str[i]
		if i == 8 || i == 13 || i == 18 || i == 23 {
			if c != '-' {
				return false
			}
		} else if !(('0' <= c && c <= '9') || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')) {
			return false
		}
	}
	return true
}

type Tag struct {
	Key   string
	Value string
}

type Log struct {
	Timestamp int64             `json:"timestamp"`
	Fields    map[string]string `json:"fields"`
}

type Logs struct {
	TraceID string `json:"traceId"`
	SpanID  string `json:"spanId"`
	Logs    []Log  `json:"logs"`
	Span    string `json:"span"`
}
