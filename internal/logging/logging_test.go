package logging

import (
	"context"
	"log/slog"
	"testing"
)

func saveDefault(t *testing.T) {
	t.Helper()
	old := slog.Default()
	t.Cleanup(func() { slog.SetDefault(old) })
}

func TestInit_LevelEnabled(t *testing.T) {
	saveDefault(t)

	tests := []struct {
		level string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"DEBUG", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"warning", slog.LevelWarn},
		{"error", slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			saveDefault(t)
			Init(tt.level, "text")
			if !slog.Default().Enabled(context.Background(), tt.want) {
				t.Errorf("level %v should be enabled after Init(%q)", tt.want, tt.level)
			}
		})
	}
}

func TestInit_DebugDisabledAtInfoLevel(t *testing.T) {
	saveDefault(t)
	Init("info", "text")
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug should not be enabled at info level")
	}
}

func TestInit_InvalidLevelDefaultsToInfo(t *testing.T) {
	saveDefault(t)
	Init("bogus", "text")
	if !slog.Default().Enabled(context.Background(), slog.LevelInfo) {
		t.Error("info should be enabled with invalid level")
	}
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		t.Error("debug should not be enabled with invalid level")
	}
}

func TestInit_JSONFormat(t *testing.T) {
	saveDefault(t)
	Init("info", "json")
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatal("Init with json format panicked")
			}
		}()
		slog.Info("test")
	}()
}

func TestInit_NoPanic(t *testing.T) {
	saveDefault(t)
	levels := []string{"debug", "info", "warn", "error", "", "unknown"}
	formats := []string{"text", "json", "", "unknown"}

	for _, l := range levels {
		for _, f := range formats {
			t.Run(l+"/"+f, func(t *testing.T) {
				saveDefault(t)
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("Init(%q, %q) panicked: %v", l, f, r)
					}
				}()
				Init(l, f)
			})
		}
	}
}
