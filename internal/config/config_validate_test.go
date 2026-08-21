package config

import (
	"strings"
	"testing"
)

func TestValidate_ValidDefault(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should validate: %v", err)
	}
}

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*Config)
		wantSub string
	}{
		{"bad port", func(c *Config) { c.Server.Port = 0 }, "port"},
		{"empty driver", func(c *Config) { c.Database.Driver = "" }, "driver"},
		{"empty dsn", func(c *Config) { c.Database.DSN = "" }, "dsn"},
		{"bad token ttl", func(c *Config) { c.Auth.TokenTTL = "24x" }, "token_ttl"},
		{"bad scan interval", func(c *Config) { c.Discovery.ScanInterval = "6" }, "scan_interval"},
		{"bad scan timeout", func(c *Config) { c.Discovery.Timeout = "abc" }, "timeout"},
		{"zero concurrency", func(c *Config) { c.Discovery.MaxConcurrentScans = 0 }, "max_concurrent_scans"},
		{"zero queue", func(c *Config) { c.Discovery.QueueSize = 0 }, "queue_size"},
		{"short custom secret", func(c *Config) { c.Auth.Secret = "too-short" }, "auth.secret"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Default()
			tc.mutate(&cfg)
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not mention %q", err, tc.wantSub)
			}
		})
	}
}

func TestWarnings(t *testing.T) {
	cfg := Default()
	warns := cfg.Warnings()
	found := false
	for _, w := range warns {
		if strings.Contains(w, "default JWT secret") {
			found = true
		}
	}
	if !found {
		t.Error("expected default-secret warning")
	}

	cfg.Auth.Secret = strings.Repeat("x", 40)
	for i := range cfg.Notifications.Profiles {
		cfg.Notifications.Profiles[i].Enabled = false
	}
	if warns := cfg.Warnings(); len(warns) != 0 {
		t.Errorf("fully configured config should have no warnings, got %v", warns)
	}

	// Custom secret shorter than 32 chars is a hard error, not a warning.
	cfg.Auth.Secret = "still-too-short"
	if err := cfg.Validate(); err == nil {
		t.Error("expected validation error for short custom secret")
	}
}
