package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// DefaultPath is the default location of the YAML configuration file,
// relative to the working directory.
const DefaultPath = "config/default.yaml"

type Config struct {
	Server        ServerConfig        `yaml:"server"`
	Database      DatabaseConfig      `yaml:"database"`
	Logging       LoggingConfig       `yaml:"logging"`
	Auth          AuthConfig          `yaml:"auth"`
	Discovery     DiscoveryConfig     `yaml:"discovery"`
	Notifications NotificationsConfig `yaml:"notifications"`
}

type ServerConfig struct {
	Host               string   `yaml:"host"`
	Port               int      `yaml:"port"`
	CORSAllowedOrigins []string `yaml:"cors_allowed_origins"`
	RateLimit          int      `yaml:"rate_limit"`
	RateLimitWindow    string   `yaml:"rate_limit_window"`
	ReadRateLimit      int      `yaml:"read_rate_limit"`
	RequestTimeout     string   `yaml:"request_timeout"`
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
	ScanInterval      string `yaml:"scan_interval"`
	Timeout           string `yaml:"timeout"`
	MaxConcurrentScans int   `yaml:"max_concurrent_scans"`
	QueueSize          int   `yaml:"queue_size"`
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
	Timezone   string   `yaml:"timezone,omitempty"`
}

func Default() Config {
	return Config{
		Server: ServerConfig{
			Host:               "0.0.0.0",
			Port:               8080,
			CORSAllowedOrigins: []string{"http://localhost:8080", "http://127.0.0.1:8080"},
			RateLimit:          10,
			RateLimitWindow:    "1m",
			ReadRateLimit:      300,
			RequestTimeout:     "30s",
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
			ScanInterval:      "6h",
			Timeout:           "30s",
			MaxConcurrentScans: 3,
			QueueSize:          100,
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
				{Name: "Management", Enabled: true, Type: "daily-digest", Recipients: []string{"manager@example.com"}, SendAt: "08:00", Timezone: "America/New_York"},
				{Name: "Security", Enabled: true, Type: "weekly-digest", Recipients: []string{"security@example.com"}, SendAt: "09:00", Day: "Monday", Timezone: "America/New_York"},
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
	if v := os.Getenv("CERTWATCH_DISCOVERY_MAX_CONCURRENT_SCANS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Discovery.MaxConcurrentScans = n
		}
	}
	if v := os.Getenv("CERTWATCH_DISCOVERY_QUEUE_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Discovery.QueueSize = n
		}
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
	if v := os.Getenv("CERTWATCH_SMTP_FORCE_TLS"); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			cfg.Notifications.SMTP.ForceTLS = b
		}
	}
	if v := os.Getenv("CERTWATCH_SERVER_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.RateLimit = n
		}
	}
	if v := os.Getenv("CERTWATCH_SERVER_RATE_LIMIT_WINDOW"); v != "" {
		cfg.Server.RateLimitWindow = v
	}
	if v := os.Getenv("CERTWATCH_SERVER_READ_RATE_LIMIT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Server.ReadRateLimit = n
		}
	}
	if v := os.Getenv("CERTWATCH_SERVER_REQUEST_TIMEOUT"); v != "" {
		cfg.Server.RequestTimeout = v
	}
	return cfg
}

func (c Config) ServerAddr() string {
	return fmt.Sprintf("%s:%d", c.Server.Host, c.Server.Port)
}

// LogValue implements slog.LogValuer so the configuration can be logged with
// slog.Info("config", "config", cfg); secrets are redacted automatically.
func (c Config) LogValue() slog.Value {
	return slog.GroupValue(
		slog.Group("server",
			slog.String("host", c.Server.Host),
			slog.Int("port", c.Server.Port),
			slog.Int("rate_limit", c.Server.RateLimit),
			slog.String("rate_limit_window", c.Server.RateLimitWindow),
			slog.Int("read_rate_limit", c.Server.ReadRateLimit),
			slog.String("request_timeout", c.Server.RequestTimeout),
		),
		slog.Group("database",
			slog.String("driver", c.Database.Driver),
			slog.String("dsn", c.Database.DSN),
		),
		slog.Group("logging",
			slog.String("level", c.Logging.Level),
			slog.String("format", c.Logging.Format),
		),
		slog.Group("auth",
			slog.String("secret", Redacted(c.Auth.Secret)),
			slog.String("token_ttl", c.Auth.TokenTTL),
		),
		slog.Group("discovery",
			slog.String("scan_interval", c.Discovery.ScanInterval),
			slog.String("timeout", c.Discovery.Timeout),
			slog.Int("max_concurrent_scans", c.Discovery.MaxConcurrentScans),
			slog.Int("queue_size", c.Discovery.QueueSize),
		),
		slog.Group("notifications",
			slog.Group("smtp",
				slog.String("host", c.Notifications.SMTP.Host),
				slog.Int("port", c.Notifications.SMTP.Port),
				slog.String("username", c.Notifications.SMTP.Username),
				slog.String("password", Redacted(c.Notifications.SMTP.Password)),
				slog.String("from", c.Notifications.SMTP.From),
				slog.Bool("force_tls", c.Notifications.SMTP.ForceTLS),
			),
			slog.Int("profiles", len(c.Notifications.Profiles)),
		),
	)
}

// Redacted masks a secret for logging: empty stays empty, anything else
// becomes "<redacted>".
func Redacted(s string) string {
	if s == "" {
		return ""
	}
	return "<redacted>"
}

// DefaultSecret is the built-in development JWT secret; production setups
// must override it.
const DefaultSecret = "change-me-in-production"

// Validate fails fast on configuration that would prevent the server from
// working correctly. It is called once during startup, before anything is
// bound or opened.
func (c Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port %d out of range [1,65535]", c.Server.Port)
	}
	if c.Database.Driver == "" {
		return fmt.Errorf("database.driver is required")
	}
	if c.Database.DSN == "" {
		return fmt.Errorf("database.dsn is required")
	}

	for name, v := range map[string]string{
		"auth.token_ttl":            c.Auth.TokenTTL,
		"discovery.scan_interval":   c.Discovery.ScanInterval,
		"discovery.timeout":         c.Discovery.Timeout,
		"server.rate_limit_window":  c.Server.RateLimitWindow,
		"server.request_timeout":    c.Server.RequestTimeout,
	} {
		if v == "" {
			continue
		}
		if _, err := time.ParseDuration(v); err != nil {
			return fmt.Errorf("%s: invalid duration %q", name, v)
		}
	}

	if c.Discovery.MaxConcurrentScans < 1 {
		return fmt.Errorf("discovery.max_concurrent_scans must be >= 1, got %d", c.Discovery.MaxConcurrentScans)
	}
	if c.Discovery.QueueSize < 1 {
		return fmt.Errorf("discovery.queue_size must be >= 1, got %d", c.Discovery.QueueSize)
	}
	if c.Notifications.SMTP.Port < 0 || c.Notifications.SMTP.Port > 65535 {
		return fmt.Errorf("notifications.smtp.port %d out of range", c.Notifications.SMTP.Port)
	}

	// A user-supplied secret shorter than 32 chars is brute-forceable; the
	// built-in default keeps its dedicated startup warning instead.
	if c.Auth.Secret != DefaultSecret && len(c.Auth.Secret) < 32 {
		return fmt.Errorf("auth.secret too short (need >= 32 characters)")
	}

	return nil
}

// Warnings reports non-fatal configuration concerns worth surfacing at
// startup.
func (c Config) Warnings() []string {
	var warns []string
	if c.Auth.Secret == DefaultSecret {
		warns = append(warns, "using default JWT secret — set CERTWATCH_AUTH_SECRET in production")
	}
	hasEnabledProfiles := false
	for _, p := range c.Notifications.Profiles {
		if p.Enabled {
			hasEnabledProfiles = true
			break
		}
	}
	if hasEnabledProfiles && c.Notifications.SMTP.Host == "" {
		warns = append(warns, "notification profiles are enabled but notifications.smtp.host is empty — emails cannot be delivered")
	}
	switch strings.ToLower(c.Logging.Level) {
	case "debug", "info", "warn", "error":
	default:
		warns = append(warns, fmt.Sprintf("logging.level %q is not one of debug/info/warn/error", c.Logging.Level))
	}
	return warns
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


