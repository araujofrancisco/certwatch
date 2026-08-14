package ctsearch

import (
	"testing"
)

func TestNormalizeDNCanonicalizesAttributeOrder(t *testing.T) {
	a := "CN=RapidSSL TLS RSA CA G1,OU=www.digicert.com,O=DigiCert Inc,C=US"
	b := "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1"
	na, nb := NormalizeDN(a), NormalizeDN(b)
	if na != nb {
		t.Fatalf("expected equivalent DNs to normalize equal:\n a=%q\n b=%q", na, nb)
	}
	want := "C=US, O=DigiCert Inc, OU=www.digicert.com, CN=RapidSSL TLS RSA CA G1"
	if na != want {
		t.Fatalf("unexpected normalized DN %q, want %q", na, want)
	}
}

func TestNormalizeDNEmpty(t *testing.T) {
	if got := NormalizeDN(""); got != "" {
		t.Errorf("expected empty for empty input, got %q", got)
	}
	if got := NormalizeDN("   "); got != "" {
		t.Errorf("expected empty for whitespace, got %q", got)
	}
}

func TestNormalizeDNUnknownAttributesPreserved(t *testing.T) {
	a := NormalizeDN("CN=foo,O=Bar,emailAddress=ops@example.com")
	b := NormalizeDN("O=Bar,emailAddress=ops@example.com,CN=foo")
	if a != b {
		t.Fatalf("expected unknown attributes to be preserved and ordered consistently:\n a=%q\n b=%q", a, b)
	}
}

func TestNormalizeDNQuotedValues(t *testing.T) {
	a := NormalizeDN(`O="DigiCert, Inc.",CN=foo`)
	b := NormalizeDN(`CN=foo,O="DigiCert, Inc."`)
	if a != b {
		t.Fatalf("expected quoted comma values to normalize equal:\n a=%q\n b=%q", a, b)
	}
}

func TestNormalizeDNWhitespace(t *testing.T) {
	a := NormalizeDN("  CN=foo ,  O=Bar  ")
	b := NormalizeDN("O=Bar,CN=foo")
	if a != b {
		t.Fatalf("expected whitespace-insensitive normalization:\n a=%q\n b=%q", a, b)
	}
}

func TestNormalizeSerial(t *testing.T) {
	cases := []struct{ in, want string }{
		{"04:AA:BB", "04aabb"},
		{"04aabb", "04aabb"},
		{"0FAB12", "0fab12"},
		{" 00:01 :02 ", "000102"},
		{"", ""},
	}
	for _, c := range cases {
		if got := NormalizeSerial(c.in); got != c.want {
			t.Errorf("NormalizeSerial(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestNormalizeDNCaseInsensitiveKeys(t *testing.T) {
	a := NormalizeDN("cn=foo,o=Bar")
	b := NormalizeDN("O=Bar,CN=foo")
	if a != b {
		t.Fatalf("expected case-insensitive attribute keys:\n a=%q\n b=%q", a, b)
	}
}