package ctsearch

import (
	"strings"
	"time"
)

// splitNames splits a provider `name_value` field (newline separated DNS names)
// into a trimmed, de-duplicated list.
func splitNames(nameValue string) []string {
	if strings.TrimSpace(nameValue) == "" {
		return nil
	}
	seen := make(map[string]struct{})
	var out []string
	for _, part := range strings.Split(nameValue, "\n") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		lc := strings.ToLower(part)
		if _, ok := seen[lc]; ok {
			continue
		}
		seen[lc] = struct{}{}
		out = append(out, lc)
	}
	return out
}

// parseCTTime parses the two date layouts used by CT search providers
// (YYYY-MM-DDTHH:MM:SS and YYYY-MM-DD). It returns the zero time when the
// value cannot be parsed.
func parseCTTime(s string) time.Time {
	for _, layout := range []string{"2006-01-02T15:04:05Z07:00", "2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// NormalizeSerial strips formatting so serials compare cleanly across providers
// (CertSpotter returns colon-separated hex; ctlogs.dev omits colons already;
// the HTTPS scanner emits bare lowercase hex). All variants collapse to
// lowercase bare hex.
func NormalizeSerial(s string) string {
	s = strings.ReplaceAll(s, ":", "")
	s = strings.ReplaceAll(s, " ", "")
	return strings.ToLower(strings.TrimSpace(s))
}