package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var durationRe = regexp.MustCompile(`^(\d+(?:\.\d+)?)\s*(ms|s|m|h|d)$`)

// parseDuration parses a duration string (e.g. "900s", "15m", "2h", "1d") into
// seconds. Returns an error if the value is not a recognized duration.
func parseDuration(value string) (float64, error) {
	m := durationRe.FindStringSubmatch(strings.TrimSpace(value))
	if m == nil {
		return 0, fmt.Errorf("invalid duration: %s", value)
	}
	num, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid duration: %s", value)
	}
	mult := map[string]float64{"ms": 0.001, "s": 1, "m": 60, "h": 3600, "d": 86400}
	return num * mult[m[2]], nil
}

// parseAttrValue coerces a raw attribute string into bool, int, float64, a
// duration (seconds as float64), or the trimmed string, mirroring the Python
// parse_attr_value precedence.
func parseAttrValue(value string) any {
	v := strings.Trim(strings.TrimSpace(value), `"`)
	switch strings.ToLower(v) {
	case "true":
		return true
	case "false":
		return false
	}
	if i, err := strconv.Atoi(v); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		return f
	}
	if d, err := parseDuration(v); err == nil {
		return d
	}
	return v
}

// --- typed attribute coercion helpers (operate on raw stored strings) ---

func attrBool(raw string, ok bool) bool {
	if !ok {
		return false
	}
	switch v := parseAttrValue(raw).(type) {
	case bool:
		return v
	case int:
		return v != 0
	case float64:
		return v != 0
	default:
		return false
	}
}

func attrInt(raw string, ok bool, def int) int {
	if !ok {
		return def
	}
	if i, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
		return i
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64); err == nil {
		return int(f)
	}
	return def
}
