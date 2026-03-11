package textutil

import "unicode/utf8"

// TruncateRunes returns s truncated to at most maxRunes Unicode code points.
func TruncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	cut, truncated := byteIndexAfterRunes(s, maxRunes)
	if !truncated {
		return s
	}
	return s[:cut]
}

// TruncateRunesWithSuffix returns s truncated to at most maxRunes Unicode code
// points, appending suffix when truncation occurs.
func TruncateRunesWithSuffix(s string, maxRunes int, suffix string) string {
	if maxRunes <= 0 || s == "" {
		return ""
	}
	if _, truncated := byteIndexAfterRunes(s, maxRunes); !truncated {
		return s
	}
	if suffix == "" {
		return TruncateRunes(s, maxRunes)
	}
	suffixRunes := utf8.RuneCountInString(suffix)
	if suffixRunes >= maxRunes {
		return TruncateRunes(s, maxRunes)
	}
	cut, truncated := byteIndexAfterRunes(s, maxRunes-suffixRunes)
	if !truncated {
		return s
	}
	return s[:cut] + suffix
}

func byteIndexAfterRunes(s string, maxRunes int) (int, bool) {
	if maxRunes <= 0 || s == "" {
		return 0, len(s) > 0
	}
	count := 0
	for i := range s {
		if count == maxRunes {
			return i, true
		}
		count++
	}
	return len(s), false
}
