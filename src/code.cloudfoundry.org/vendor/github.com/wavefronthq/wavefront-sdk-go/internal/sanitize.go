package internal

import (
	"regexp"
	"strings"
)

var /* const */ quotation = regexp.MustCompile(`"`)
var /* const */ lineBreak = regexp.MustCompile(`\n`)

// Sanitize sanitizes string of metric name, source and key of tags according to the rule of Wavefront proxy.
func Sanitize(str string) string {
	sb := GetBuffer()
	defer PutBuffer(sb)

	// first character can be \u2206 (∆ - INCREMENT) or \u0394 (Δ - GREEK CAPITAL LETTER DELTA)
	// or ~ tilda character for internal metrics
	skipHead := 0
	if strings.HasPrefix(str, DeltaPrefix) {
		sb.WriteString(DeltaPrefix)
		skipHead = 3
	}
	if strings.HasPrefix(str, AltDeltaPrefix) {
		sb.WriteString(AltDeltaPrefix)
		skipHead = 2
	}
	// Second character can be ~ tilda character if first character
	// is \u2206 (∆ - INCREMENT) or \u0394 (Δ - GREEK CAPITAL LETTER)
	if (strings.HasPrefix(str, DeltaPrefix) || strings.HasPrefix(str, AltDeltaPrefix)) &&
		str[skipHead] == 126 {
		sb.WriteString(string(str[skipHead]))
		skipHead++
	}
	if str[0] == 126 {
		sb.WriteString(string(str[0]))
		skipHead = 1
	}

	for i := 0; i < len(str); i++ {
		if skipHead > 0 {
			i += skipHead
			skipHead = 0
		}
		cur := str[i]
		strCur := string(cur)
		isLegal := true

		if !(44 <= cur && cur <= 57) && !(65 <= cur && cur <= 90) && !(97 <= cur && cur <= 122) && cur != 95 {
			isLegal = false
		}
		if isLegal {
			sb.WriteString(strCur)
		} else {
			sb.WriteString("-")
		}
	}
	return sb.String()
}

// SanitizeValue sanitizes string of tags value, etc.
func SanitizeValue(str string) string {
	res := strings.TrimSpace(str)
	if strings.Contains(str, "\"") || strings.Contains(str, "'") {
		res = quotation.ReplaceAllString(res, "\\\"")
	}
	return "\"" + lineBreak.ReplaceAllString(res, "\\n") + "\""
}
