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
	Server   ServerConfig   `mapstructure:"server"`
	Database DatabaseConfig `mapstructure:"database"`
	Admin    AdminConfig    `mapstructure:"admin"`
	LogLevel string         `mapstructure:"log_level"`
}

// ServerConfig holds the settings of the HTTP server started by `serve`.
type ServerConfig struct {
	Port            int           `mapstructure:"port"`
	ReadTimeout     time.Duration `mapstructure:"read_timeout"`
	WriteTimeout    time.Duration `mapstructure:"write_timeout"`
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout"`
}

// DatabaseConfig holds the settings of the PostgreSQL connection. Pop is
// connected from these values rather than from a database.yml, so the database
// obeys the same Config Source ranking as everything else (see ADR-0005).
type DatabaseConfig struct {
	URL             DSN           `mapstructure:"url"`
	Pool            int           `mapstructure:"pool"`
	IdlePool        int           `mapstructure:"idle_pool"`
	ConnMaxLifetime time.Duration `mapstructure:"conn_max_lifetime"`
}

// AdminConfig holds everything the Admin surface needs that is not content.
//
// The Author is a config value rather than a database row (ADR-0009), so their
// credential lives here, and so does the key their session is signed with.
type AdminConfig struct {
	// PasswordHash is an argon2id encoded hash ($argon2id$v=19$m=...$salt$hash).
	// The parameters are read from the hash itself at verification, never from
	// here: a hash carries the parameters it was made with, and verifying
	// against different ones fails for every password including the right one.
	PasswordHash Secret `mapstructure:"password_hash"`
	// SessionSecret keys the HMAC of the session cookie. Rotating it
	// invalidates every outstanding session, which is the intended way to log
	// every device out.
	SessionSecret Secret `mapstructure:"session_secret"`
	// SessionTTL is how long a session lasts from the moment it is issued.
	// There is no renewal: without session storage there is nothing to revoke
	// against, so a sliding window would be extended by a stolen cookie's own
	// use and never close (ADR-0015).
	SessionTTL time.Duration `mapstructure:"session_ttl"`
	// InsecureCookie drops the Secure attribute so that Admin can be used over
	// plain HTTP in development. It is an explicit key rather than a guess at
	// whether the host looks like localhost, because that guess is how a
	// production deployment quietly stops setting Secure.
	InsecureCookie bool `mapstructure:"insecure_cookie"`
	// Timezone is the zone a Publication Date typed into the edit form is
	// understood in, and the zone every date is displayed in. It is stated
	// rather than taken from the server, so moving a deployment cannot shift a
	// schedule.
	Timezone string `mapstructure:"timezone"`
	// LoginAttempts is how many login attempts one address may make within
	// LoginWindow before being refused. The limit stands in front of argon2id,
	// which is expensive by design and therefore something an unauthenticated
	// caller must not be able to spend freely (ADR-0015).
	LoginAttempts int `mapstructure:"login_attempts"`
	// LoginWindow is the period LoginAttempts is counted over.
	LoginWindow time.Duration `mapstructure:"login_window"`
}

// Enabled reports whether Admin has been given the two secrets it cannot run
// without. When it has not, `serve` mounts the public site alone and says so;
// a personal machine that has never set them still gets a working `make run`.
func (a AdminConfig) Enabled() bool {
	return a.PasswordHash.Set() && a.SessionSecret.Set()
}

// Location resolves Timezone. Validate has already accepted it, so an unknown
// name here falls back to UTC rather than failing.
func (a AdminConfig) Location() *time.Location {
	loc, err := time.LoadLocation(a.Timezone)
	if err != nil {
		return time.UTC
	}
	return loc
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
	if err := c.Database.URL.validate(); err != nil {
		return err
	}
	if c.Database.Pool < 1 {
		return fmt.Errorf("database.pool must be at least 1, got %d", c.Database.Pool)
	}
	if c.Database.IdlePool < 0 {
		return fmt.Errorf("database.idle_pool must not be negative, got %d", c.Database.IdlePool)
	}
	if c.Database.IdlePool > c.Database.Pool {
		return fmt.Errorf("database.idle_pool (%d) must not exceed database.pool (%d)",
			c.Database.IdlePool, c.Database.Pool)
	}
	if c.Database.ConnMaxLifetime <= 0 {
		return fmt.Errorf("database.conn_max_lifetime must be greater than zero, got %s",
			c.Database.ConnMaxLifetime)
	}
	if err := c.Admin.validate(); err != nil {
		return err
	}
	if _, ok := logLevels[c.LogLevel]; !ok {
		return fmt.Errorf("log_level must be one of debug, info, warn, error, got %q", c.LogLevel)
	}
	return nil
}

// validate checks the Admin settings that are always meaningful. The two
// secrets are not among them: absent, Admin is simply not mounted, and
// demanding them would make a fresh clone fail to serve the public site.
func (a AdminConfig) validate() error {
	if a.SessionTTL <= 0 {
		return fmt.Errorf("admin.session_ttl must be greater than zero, got %s", a.SessionTTL)
	}
	if _, err := time.LoadLocation(a.Timezone); err != nil {
		return fmt.Errorf("admin.timezone is not a known zone: %q", a.Timezone)
	}
	if a.LoginAttempts < 1 {
		return fmt.Errorf("admin.login_attempts must be at least 1, got %d", a.LoginAttempts)
	}
	if a.LoginWindow <= 0 {
		return fmt.Errorf("admin.login_window must be greater than zero, got %s", a.LoginWindow)
	}
	// A password hash without a signing key (or the reverse) is a deployment
	// that believes Admin is protected when it is simply absent. Saying so is
	// the whole reason config rejects what it cannot honour.
	if a.PasswordHash.Set() != a.SessionSecret.Set() {
		return fmt.Errorf("admin.password_hash and admin.session_secret must be set together")
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
		// URL renders through DSN.MarshalYAML, so the password never reaches
		// stdout however this map is printed.
		"database": map[string]any{
			"url":               c.Database.URL,
			"pool":              c.Database.Pool,
			"idle_pool":         c.Database.IdlePool,
			"conn_max_lifetime": c.Database.ConnMaxLifetime.String(),
		},
		// Both admin secrets render through Secret.MarshalYAML, so neither
		// reaches stdout however this map is printed.
		"admin": map[string]any{
			"password_hash":   c.Admin.PasswordHash,
			"session_secret":  c.Admin.SessionSecret,
			"session_ttl":     c.Admin.SessionTTL.String(),
			"insecure_cookie": c.Admin.InsecureCookie,
			"timezone":        c.Admin.Timezone,
			"login_attempts":  c.Admin.LoginAttempts,
			"login_window":    c.Admin.LoginWindow.String(),
		},
	}, nil
}
