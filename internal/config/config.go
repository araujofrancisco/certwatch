package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Logging       LoggingConfig       `yaml:"logging"`
	Auth          AuthConfig          `yaml:"auth"`
	Discovery     DiscoveryConfig     `yaml:"discovery"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

type ServerConfig struct {
	Host              string   `yaml:"host"`
	Port              int      `yaml:"port"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
}

type DatabaseConfig struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type AuthConfig struct {
	Secret   string `yaml:"secret"`
	TokenTTL string `yaml:"token_ttl"`
}

type DiscoveryConfig struct {
	ScanInterval string `yaml:"scan_interval"`
	Timeout      string `yaml:"timeout"`
}

type NotificationsConfig struct {
	SMTP     SMTPConfig       `yaml:"smtp"`
	Profiles []ProfileConfig  `yaml:"profiles"`
}

type SMTPConfig struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
	ForceTLS bool   `yaml:"force_tls"`
}

type ProfileConfig struct {
	Name       string   `yaml:"name"`
	Enabled    bool     `yaml:"enabled"`
	Type       string   `yaml:"type"`
	Recipients []string `yaml:"recipients"`
	Thresholds []int    `yaml:"thresholds,omitempty"`
	SendAt     string   `yaml:"send_at,omitempty"`
	Day        string   `yaml:"day,omitempty"`
	Cron       string   `yaml:"cron,omitempty"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:              "0.0.0.0",
			Port:              8080,
			CORSAllowedOrigins: []string{"http://localhost:8080", "http://127.0.0.1:8080"},
		},
		Database: DatabaseConfig{
			Driver: "sqlite",
			DSN:    "certwatch.db",
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
		Auth: AuthConfig{
			Secret:   "change-me-in-production",
			TokenTTL: "24h",
		},
		Discovery: DiscoveryConfig{
			ScanInterval: "6h",
			Timeout:      "30s",
		},
		Notifications: NotificationsConfig{
			SMTP: SMTPConfig{
				Host:     "",
				Port:     587,
				Username: "",
				Password: "",
				From:     "",
			},
			Profiles: []ProfileConfig{
				{Name: "Operations", Enabled: true, Type: "immediate", Recipients: []string{"ops@example.com"}, Thresholds: []int{30, 14, 7, 3, 1}},
				{Name: "Management", Enabled: true, Type: "daily-digest", Recipients: []string{"manager@example.com"}, SendAt: "08:00"},
				{Name: "Security", Enabled: true, Type: "weekly-digest", Recipients: []string{"security@example.com"}, SendAt: "09:00", Day: "Monday"},
			},
		},
	}
}

func Load(path string) (Config, error) {
	cfg := Default()

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return applyEnvOverrides(cfg), nil
		}
		return cfg, fmt.Errorf("read config file: %w", err)
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, fmt.Errorf("parse config file: %w", err)
	}

	return applyEnvOverrides(cfg), nil
}

func applyEnvOverrides(cfg Config) Config {
	if v := os.Getenv("CERTWATCH_SERVER_HOST"); v != "" {
		cfg.Server.Host = v
	}
	if v := os.Getenv("CERTWATCH_SERVER_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Server.Port = p
		}
	}
	if v := os.Getenv("CERTWATCH_DATABASE_DRIVER"); v != "" {
		cfg.Database.Driver = v
	}
	if v := os.Getenv("CERTWATCH_DATABASE_DSN"); v != "" {
		cfg.Database.DSN = v
	}
	if v := os.Getenv("CERTWATCH_LOGGING_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("CERTWATCH_LOGGING_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}
	if v := os.Getenv("CERTWATCH_AUTH_SECRET"); v != "" {
		cfg.Auth.Secret = v
	}
	if v := os.Getenv("CERTWATCH_AUTH_TOKEN_TTL"); v != "" {
		cfg.Auth.TokenTTL = v
	}
	if v := os.Getenv("CERTWATCH_DISCOVERY_SCAN_INTERVAL"); v != "" {
		cfg.Discovery.ScanInterval = v
	}
	if v := os.Getenv("CERTWATCH_DISCOVERY_TIMEOUT"); v != "" {
		cfg.Discovery.Timeout = v
	}
	if v := os.Getenv("CERTWATCH_SMTP_HOST"); v != "" {
		cfg.Notifications.SMTP.Host = v
	}
	if v := os.Getenv("CERTWATCH_SMTP_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Notifications.SMTP.Port = p
		}
	}
	if v := os.Getenv("CERTWATCH_SMTP_USERNAME"); v != "" {
		cfg.Notifications.SMTP.Username = v
	}
	if v := os.Getenv("CERTWATCH_SMTP_PASSWORD"); v != "" {
		cfg.Notifications.SMTP.Password = v
	}
	if v := os.Getenv("CERTWATCH_SERVER_CORS_ORIGINS"); v != "" {
		origins := []string{}
		for _, o := range splitAndTrim(v, ",") {
			if o != "" {
				origins = append(origins, o)
			}
		}
		cfg.Server.CORSAllowedOrigins = origins
	}
	if v := os.Getenv("CERTWATCH_SMTP_FROM"); v != "" {
		cfg.Notifications.SMTP.From = v
	}
	return cfg
}

func (c Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

func splitAndTrim(s, sep string) []string {
	var result []string
	for _, part := range strings.Split(s, sep) {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}


