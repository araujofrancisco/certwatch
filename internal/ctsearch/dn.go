package ctsearch

import (
	"strings"
)

// canonicalAttrOrder ranks issuer DN attributes so the same CA renders as one
// canonical string regardless of how a provider orders the RDNs.
var canonicalAttrOrder = []string{"C", "ST", "L", "O", "OU", "CN"}

// NormalizeDN canonicalizes a distinguished name string so DNs that differ only
// in attribute order or whitespace compare equal. Providers render the same
// issuer in different attribute orders (ctlogs.dev emits CN-first, CertSpotter
// emits C-first); normalizing here makes Entry.Key/Fingerprint and the service
// dedup stable across providers.
//
// Each RDN is emitted as "Key=Value" in canonical attribute order (C, ST, L,
// O, OU, CN, then any unknown attributes in original order), joined with ", ".
// Values are trimmed and surrounding quotes removed; an unparseable string is
// returned trimmed.
func NormalizeDN(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}

	rdns := splitRDNs(s)
	if len(rdns) == 0 {
		return s
	}

	// Preserve original order for unknown attributes.
	var extras []string
	byKey := make(map[string]string)
	var order []string
	for _, rdn := range rdns {
		key, value, ok := splitRDN(rdn)
		if !ok {
			// Unparseable component: keep it verbatim rather than dropping it.
			extras = append(extras, normalizeValue(rdn))
			continue
		}
		key = normalizeKey(key)
		value = normalizeValue(value)
		if _, exists := byKey[key]; exists {
			continue
		}
		byKey[key] = value
		order = append(order, key)
	}

	var parts []string
	for _, key := range canonicalAttrOrder {
		if value, ok := byKey[key]; ok {
			parts = append(parts, key+"="+value)
		}
	}
	for _, key := range order {
		if isCanonical(key) {
			continue
		}
		parts = append(parts, key+"="+byKey[key])
	}
	parts = append(parts, extras...)

	return strings.Join(parts, ", ")
}

// splitRDNs splits a DN into its RDN components on unescaped, unquoted commas.
func splitRDNs(s string) []string {
	var parts []string
	var cur strings.Builder
	inQuote := false
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			cur.WriteRune(r)
			escaped = true
		case r == '"':
			inQuote = !inQuote
			cur.WriteRune(r)
		case r == ',' && !inQuote:
			parts = append(parts, strings.TrimSpace(cur.String()))
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}
	return parts
}

// splitRDN splits one RDN ("O=DigiCert Inc") into key and value on the first
// unquoted "=". It returns ok=false when no key/value separator is found.
func splitRDN(rdn string) (key, value string, ok bool) {
	idx := indexUnquoted(rdn, '=')
	if idx < 0 {
		return "", "", false
	}
	return rdn[:idx], rdn[idx+1:], true
}

// indexUnquoted returns the index of the first occurrence of c outside quotes
// and not preceded by a backslash, or -1 when absent.
func indexUnquoted(s string, c byte) int {
	inQuote := false
	escaped := false
	for i := 0; i < len(s); i++ {
		b := s[i]
		switch {
		case escaped:
			escaped = false
		case b == '\\':
			escaped = true
		case b == '"':
			inQuote = !inQuote
		case b == c && !inQuote:
			return i
		}
	}
	return -1
}

// normalizeKey uppercases an attribute key (issuer keys are case-insensitive).
func normalizeKey(k string) string {
	return strings.ToUpper(strings.TrimSpace(k))
}

// normalizeValue trims whitespace, collapses interior whitespace runs, and
// strips a single pair of surrounding quotes.
func normalizeValue(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		v = v[1 : len(v)-1]
	}
	return strings.Join(strings.Fields(v), " ")
}

// isCanonical reports whether key is part of the canonical ordering.
func isCanonical(key string) bool {
	for _, k := range canonicalAttrOrder {
		if k == key {
			return true
		}
	}
	return false
}