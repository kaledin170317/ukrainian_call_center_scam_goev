package billing

import "strings"

func splitExact(s string, sep byte, out []string) bool {
	i := 0
	start := 0
	for j := 0; j < len(s); j++ {
		if s[j] == sep {
			if i >= len(out) {
				return false
			}
			out[i] = strings.TrimSpace(s[start:j])
			i++
			start = j + 1
		}
	}
	if i != len(out)-1 {
		return false
	}
	out[i] = strings.TrimSpace(s[start:])
	return true
}

func unquoteLoose(s string) string {
	if len(s) >= 2 {
		if (s[0] == '`' && s[len(s)-1] == '`') ||
			(s[0] == '\'' && s[len(s)-1] == '\'') ||
			(s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
