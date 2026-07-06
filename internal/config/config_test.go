package config

import (
	"os"
	"testing"
)

func TestDefault(t *testing.T) {
	cfg := Default()
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "sqlite" {
		t.Errorf("expected sqlite driver, got %s", cfg.Database.Driver)
	}
}

func TestLoad_MissingFile(t *testing.T) {
	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port, got %d", cfg.Server.Port)
	}
}

func TestEnvOverrides(t *testing.T) {
	os.Setenv("CERTWATCH_SERVER_PORT", "9090")
	os.Setenv("CERTWATCH_LOGGING_LEVEL", "debug")
	defer func() {
		os.Unsetenv("CERTWATCH_SERVER_PORT")
		os.Unsetenv("CERTWATCH_LOGGING_LEVEL")
	}()

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected port 9090, got %d", cfg.Server.Port)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected debug level, got %s", cfg.Logging.Level)
	}
}

func TestServerAddr(t *testing.T) {
	cfg := Default()
	if addr := cfg.ServerAddr(); addr != "0.0.0.0:8080" {
		t.Errorf("expected 0.0.0.0:8080, got %s", addr)
	}
}

func TestLoad_ValidFile(t *testing.T) {
	content := []byte(`
server:
  host: "127.0.0.1"
  port: 9090
logging:
  level: debug
`)
	tmp, _ := os.CreateTemp("", "config-*.yaml")
	defer os.Remove(tmp.Name())
	tmp.Write(content)
	tmp.Close()

	cfg, err := Load(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("expected 127.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected 9090, got %d", cfg.Server.Port)
	}
}

func TestLoad_MalformedYAML(t *testing.T) {
	tmp, _ := os.CreateTemp("", "config-*.yaml")
	defer os.Remove(tmp.Name())
	tmp.WriteString("{{{{{invalid yaml")
	tmp.Close()

	_, err := Load(tmp.Name())
	if err == nil {
		t.Error("expected error for malformed YAML")
	}
}

func TestEnvOverrides_PortInvalid(t *testing.T) {
	os.Setenv("CERTWATCH_SERVER_PORT", "notanumber")
	defer os.Unsetenv("CERTWATCH_SERVER_PORT")

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("expected default port 8080, got %d", cfg.Server.Port)
	}
}

func TestEnvOverrides_All(t *testing.T) {
	os.Setenv("CERTWATCH_SERVER_HOST", "10.0.0.1")
	os.Setenv("CERTWATCH_SERVER_PORT", "9090")
	os.Setenv("CERTWATCH_DATABASE_DRIVER", "postgres")
	os.Setenv("CERTWATCH_DATABASE_DSN", "host=localhost")
	os.Setenv("CERTWATCH_LOGGING_LEVEL", "debug")
	os.Setenv("CERTWATCH_LOGGING_FORMAT", "json")
	defer func() {
		os.Unsetenv("CERTWATCH_SERVER_HOST")
		os.Unsetenv("CERTWATCH_SERVER_PORT")
		os.Unsetenv("CERTWATCH_DATABASE_DRIVER")
		os.Unsetenv("CERTWATCH_DATABASE_DSN")
		os.Unsetenv("CERTWATCH_LOGGING_LEVEL")
		os.Unsetenv("CERTWATCH_LOGGING_FORMAT")
	}()

	cfg, err := Load("/nonexistent/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Host != "10.0.0.1" {
		t.Errorf("expected 10.0.0.1, got %s", cfg.Server.Host)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("expected 9090, got %d", cfg.Server.Port)
	}
	if cfg.Database.Driver != "postgres" {
		t.Errorf("expected postgres, got %s", cfg.Database.Driver)
	}
	if cfg.Database.DSN != "host=localhost" {
		t.Errorf("expected host=localhost, got %s", cfg.Database.DSN)
	}
	if cfg.Logging.Level != "debug" {
		t.Errorf("expected debug, got %s", cfg.Logging.Level)
	}
	if cfg.Logging.Format != "json" {
		t.Errorf("expected json, got %s", cfg.Logging.Format)
	}
}
