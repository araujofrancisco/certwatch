package notifier

import (
	"context"
	"testing"
	"time"

	"github.com/araujofrancisco/certwatch/internal/config"
	"github.com/araujofrancisco/certwatch/internal/models"
)

func TestNewNotifier(t *testing.T) {
	cfg := config.NotificationsConfig{
		SMTP: config.SMTPConfig{Host: "localhost", Port: 1025, From: "test@example.com"},
		Profiles: []config.ProfileConfig{
			{Name: "Test", Enabled: true, Type: "immediate", Recipients: []string{"test@example.com"}, Thresholds: []int{30, 14}},
		},
	}
	n := New(cfg)
	if len(n.Profiles()) != 1 {
		t.Errorf("expected 1 profile, got %d", len(n.Profiles()))
	}
}

func TestSendEmailNoSMTP(t *testing.T) {
	cfg := config.NotificationsConfig{
		SMTP: config.SMTPConfig{Host: "", Port: 0, From: ""},
	}
	n := New(cfg)
	err := n.SendEmail(context.TODO(), []string{"test@example.com"}, "Test", "Body")
	if err != nil {
		t.Errorf("expected no error when SMTP not configured, got: %v", err)
	}
}

func TestValidateProfiles(t *testing.T) {
	tests := []struct {
		name    string
		profile config.ProfileConfig
		ok      bool
	}{
		{"valid immediate", config.ProfileConfig{Name: "ops", Enabled: true, Type: "immediate", Recipients: []string{"ops@example.com"}, Thresholds: []int{30, 14, 7, 3, 1}}, true},
		{"valid daily", config.ProfileConfig{Name: "daily", Enabled: true, Type: "daily-digest", Recipients: []string{"a@b.com"}, SendAt: "08:00"}, true},
		{"valid weekly", config.ProfileConfig{Name: "weekly", Enabled: true, Type: "weekly-digest", Recipients: []string{"a@b.com"}, SendAt: "09:00", Day: "Monday"}, true},
		{"no name", config.ProfileConfig{Enabled: true, Type: "immediate", Recipients: []string{"a@b.com"}, Thresholds: []int{30}}, false},
		{"no recipients", config.ProfileConfig{Name: "x", Enabled: true, Type: "immediate", Thresholds: []int{30}}, false},
		{"bad email", config.ProfileConfig{Name: "x", Enabled: true, Type: "immediate", Recipients: []string{"notanemail"}, Thresholds: []int{30}}, false},
		{"no thresholds immediate", config.ProfileConfig{Name: "x", Enabled: true, Type: "immediate", Recipients: []string{"a@b.com"}}, false},
		{"bad thresholds", config.ProfileConfig{Name: "x", Enabled: true, Type: "immediate", Recipients: []string{"a@b.com"}, Thresholds: []int{7, 14}}, false},
		{"daily no send_at", config.ProfileConfig{Name: "x", Enabled: true, Type: "daily-digest", Recipients: []string{"a@b.com"}}, false},
		{"weekly no day", config.ProfileConfig{Name: "x", Enabled: true, Type: "weekly-digest", Recipients: []string{"a@b.com"}, SendAt: "09:00"}, false},
		{"unknown type", config.ProfileConfig{Name: "x", Enabled: true, Type: "unknown", Recipients: []string{"a@b.com"}}, false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateProfiles([]config.ProfileConfig{tc.profile})
			if tc.ok && err != nil {
				t.Errorf("expected ok, got: %v", err)
			}
			if !tc.ok && err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestValidateDuplicates(t *testing.T) {
	profiles := []config.ProfileConfig{
		{Name: "dup", Type: "immediate", Recipients: []string{"a@b.com"}, Thresholds: []int{30}},
		{Name: "dup", Type: "immediate", Recipients: []string{"a@b.com"}, Thresholds: []int{30}},
	}
	err := ValidateProfiles(profiles)
	if err == nil {
		t.Error("expected error for duplicate names")
	}
}

func TestDefaultCron(t *testing.T) {
	daily := config.ProfileConfig{Type: "daily-digest", SendAt: "08:00"}
	weekly := config.ProfileConfig{Type: "weekly-digest", SendAt: "09:00", Day: "Monday"}

	if DefaultCron(daily) != "00 08 * * *" {
		t.Errorf("unexpected daily cron: %s", DefaultCron(daily))
	}
	if DefaultCron(weekly) != "00 09 * * 1" {
		t.Errorf("unexpected weekly cron: %s", DefaultCron(weekly))
	}
}

func TestFilterEnabled(t *testing.T) {
	profiles := []config.ProfileConfig{
		{Name: "a", Enabled: true},
		{Name: "b", Enabled: false},
		{Name: "c", Enabled: true},
	}
	enabled := FilterEnabled(profiles)
	if len(enabled) != 2 {
		t.Errorf("expected 2 enabled, got %d", len(enabled))
	}
}

func TestMatcher(t *testing.T) {
	profiles := []config.ProfileConfig{
		{Name: "ops", Enabled: true, Type: "immediate", Thresholds: []int{14, 7}},
	}
	m := NewMatcher(profiles)

	now := time.Now()
	certs := []models.Certificate{
		{DomainID: 1, NotAfter: now.AddDate(0, 0, 10)},
		{DomainID: 2, NotAfter: now.AddDate(0, 0, 20)},
		{DomainID: 3, NotAfter: now.AddDate(0, 0, 5)},
	}

	matches := m.FindMatches(certs)
	if len(matches) == 0 {
		t.Fatal("expected matches")
	}

	found := false
	for _, m := range matches {
		if m.Threshold == 7 && len(m.Certificates) > 0 {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected match for 7-day threshold")
	}
}

func TestBuildDailyDigest(t *testing.T) {
	m := NewMatcher(nil)
	now := time.Now()
	domains := []models.Domain{{ID: 1, Domain: "example.com"}}
	certs := []models.Certificate{
		{DomainID: 1, NotAfter: now.AddDate(0, 0, 30)},
		{DomainID: 1, NotAfter: now.AddDate(0, 0, 7)},
		{DomainID: 1, NotAfter: now.AddDate(0, 0, -2)},
	}

	section := m.BuildDailyDigest(certs, domains)
	if len(section.Healthy) != 1 {
		t.Errorf("expected 1 healthy, got %d", len(section.Healthy))
	}
	if len(section.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d", len(section.Warnings))
	}
	if len(section.Critical) != 1 {
		t.Errorf("expected 1 critical, got %d", len(section.Critical))
	}
}

func TestBuildWeeklyReport(t *testing.T) {
	m := NewMatcher(nil)
	now := time.Now()
	domains := []models.Domain{{ID: 1, Domain: "a.com"}, {ID: 2, Domain: "b.com"}}
	certs := []models.Certificate{
		{DomainID: 1, NotAfter: now.AddDate(0, 0, 30)},
		{DomainID: 1, NotAfter: now.AddDate(0, 0, 7)},
		{DomainID: 2, NotAfter: now.AddDate(0, 0, -1)},
	}

	report := m.BuildWeeklyReport(certs, domains)
	if report.TotalDomains != 2 {
		t.Errorf("expected 2 total, got %d", report.TotalDomains)
	}
	if report.Healthy != 1 {
		t.Errorf("expected 1 healthy, got %d", report.Healthy)
	}
	if report.Warning != 1 {
		t.Errorf("expected 1 warning, got %d", report.Warning)
	}
	if report.Expired != 1 {
		t.Errorf("expected 1 expired, got %d", report.Expired)
	}
}
