package event

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/wavefronthq/wavefront-sdk-go/event"
	"github.com/wavefronthq/wavefront-sdk-go/internal"
)

// Line encode the event to a wf proxy format
// set endMillis to 0 for a 'Instantaneous' event
func Line(name string, startMillis, endMillis int64, source string, tags map[string]string, setters ...event.Option) (string, error) {
	sb := internal.GetBuffer()
	defer internal.PutBuffer(sb)

	annotations := map[string]string{}
	l := map[string]interface{}{
		"annotations": annotations,
	}
	for _, set := range setters {
		set(l)
	}

	sb.WriteString("@Event")

	startMillis, endMillis = adjustStartEndTime(startMillis, endMillis)

	sb.WriteString(" ")
	sb.WriteString(strconv.FormatInt(startMillis, 10))
	sb.WriteString(" ")
	sb.WriteString(strconv.FormatInt(endMillis, 10))

	sb.WriteString(" ")
	sb.WriteString(strconv.Quote(name))

	for k, v := range annotations {
		sb.WriteString(" ")
		sb.WriteString(k)
		sb.WriteString("=")
		sb.WriteString(strconv.Quote(v))
	}

	if len(source) > 0 {
		sb.WriteString(" host=")
		sb.WriteString(strconv.Quote(source))
	}

	for k, v := range tags {
		sb.WriteString(" tag=")
		sb.WriteString(strconv.Quote(fmt.Sprintf("%v: %v", k, v)))
	}

	sb.WriteString("\n")
	return sb.String(), nil
}

// LineJSON encodes the event to a wf API format
func LineJSON(name string, startMillis, endMillis int64, source string, tags map[string]string, setters ...event.Option) (string, error) {
	annotations := map[string]string{}
	l := map[string]interface{}{
		"name":        name,
		"annotations": annotations,
	}

	for _, set := range setters {
		set(l)
	}

	startMillis, endMillis = adjustStartEndTime(startMillis, endMillis)

	l["startTime"] = startMillis
	l["endTime"] = endMillis

	if len(tags) > 0 {
		var tagList []string
		for k, v := range tags {
			tagList = append(tagList, fmt.Sprintf("%v: %v", k, v))
		}
		l["tags"] = tagList
	}

	if len(source) > 0 {
		l["hosts"] = []string{source}
	}

	jsonData, err := json.Marshal(l)
	if err != nil {
		return "", err
	}

	return string(jsonData), nil
}

func adjustStartEndTime(startMillis, endMillis int64) (int64, int64) {
	// secs to millis
	if startMillis < 999999999999 {
		startMillis = startMillis * 1000
	}

	if endMillis <= 999999999999 {
		endMillis = endMillis * 1000
	}

	if endMillis == 0 {
		endMillis = startMillis + 1
	}
	return startMillis, endMillis
}
