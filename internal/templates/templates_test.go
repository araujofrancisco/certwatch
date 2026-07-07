package templates

import (
	"strings"
	"testing"
	"time"
)

func TestImmediateAlert(t *testing.T) {
	info := CertInfo{
		Domain:      "example.com",
		Issuer:      "Test CA",
		Expires:     time.Date(2026, 7, 20, 0, 0, 0, 0, time.UTC),
		DaysRemains: 14,
	}
	subject, body := ImmediateAlert(info)
	if !strings.Contains(subject, "example.com") {
		t.Errorf("subject should contain domain, got: %s", subject)
	}
	if !strings.Contains(subject, "14") {
		t.Errorf("subject should contain days, got: %s", subject)
	}
	if !strings.Contains(body, "Test CA") {
		t.Errorf("body should contain issuer, got: %s", body)
	}
}

func TestDailyDigest(t *testing.T) {
	section := DailySection{
		Healthy:  []CertInfo{{Domain: "good.com", DaysRemains: 30}},
		Warnings: []CertInfo{{Domain: "warn.com", DaysRemains: 7}},
		Critical: []CertInfo{{Domain: "bad.com", DaysRemains: -2}},
	}
	subject, body := DailyDigest(time.Now(), section)
	if !strings.Contains(subject, "Daily Certificate Report") {
		t.Errorf("unexpected subject: %s", subject)
	}
	if !strings.Contains(body, "good.com") {
		t.Errorf("body should contain healthy domain")
	}
	if !strings.Contains(body, "warn.com") {
		t.Errorf("body should contain warning domain")
	}
	if !strings.Contains(body, "bad.com") {
		t.Errorf("body should contain critical domain")
	}
}

func TestWeeklyReportDigest(t *testing.T) {
	report := WeeklyReport{
		TotalDomains: 10,
		Healthy:      7,
		Warning:      2,
		Expired:      1,
	}
	subject, body := WeeklyReportDigest(report)
	if !strings.Contains(subject, "Certificate Summary") {
		t.Errorf("unexpected subject: %s", subject)
	}
	if !strings.Contains(body, "7") {
		t.Errorf("body should contain healthy count")
	}
}

func TestImmediateFormat(t *testing.T) {
	info := CertInfo{Domain: "test.com", Issuer: "CA", Expires: time.Now().AddDate(0, 0, 7), DaysRemains: 7}
	subject, body := ImmediateAlert(info)
	expected := "[Critical] test.com expires in 7 days"
	if subject != expected {
		t.Errorf("expected %q, got %q", expected, subject)
	}
	if !strings.Contains(body, "Days Remaining:\n7") {
		t.Errorf("body missing days remaining")
	}
}
