// Package config owns every Config Source and the Effective Config they
// produce. Viper does not leak past this package: the rest of the application
// receives a *Config and nothing else.
package config

import (
	"fmt"
	"log/slog"
	"time"
)

// Config is the Effective Config the application runs with.
type Config struct {
	Server   ServerConfig `mapstructure:"server"`
	LogLevel string       `mapstructure:"log_level"`
}

// ServerConfig holds the settings of the HTTP server started by `serve`.
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

var logLevels = map[string]slog.Level{
	"debug": slog.LevelDebug,
	"info":  slog.LevelInfo,
	"warn":  slog.LevelWarn,
	"error": slog.LevelError,
}

// SlogLevel reports the slog level named by LogLevel. Validate guarantees the
// name is known, so an unknown one falls back to info rather than failing here.
func (c *Config) SlogLevel() slog.Level {
	if lvl, ok := logLevels[c.LogLevel]; ok {
		return lvl
	}
	return slog.LevelInfo
}

// Validate checks the domain rules that types alone cannot express.
func (c *Config) Validate() error {
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return fmt.Errorf("server.port must be between 1 and 65535, got %d", c.Server.Port)
	}
	for name, d := range map[string]time.Duration{
		"server.read_timeout":     c.Server.ReadTimeout,
		"server.write_timeout":    c.Server.WriteTimeout,
		"server.shutdown_timeout": c.Server.ShutdownTimeout,
	} {
		if d <= 0 {
			return fmt.Errorf("%s must be greater than zero, got %s", name, d)
		}
	}
	if _, ok := logLevels[c.LogLevel]; !ok {
		return fmt.Errorf("log_level must be one of debug, info, warn, error, got %q", c.LogLevel)
	}
	return nil
}

// MarshalYAML renders the Effective Config for `config show`. Durations are
// written as strings so the output can be pasted straight back into a Config
// File instead of appearing as a nanosecond count.
func (c Config) MarshalYAML() (any, error) {
	return map[string]any{
		"log_level": c.LogLevel,
		"server": map[string]any{
			"port":             c.Server.Port,
			"read_timeout":     c.Server.ReadTimeout.String(),
			"write_timeout":    c.Server.WriteTimeout.String(),
			"shutdown_timeout": c.Server.ShutdownTimeout.String(),
		},
	}, nil
}
