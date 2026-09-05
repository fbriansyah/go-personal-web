package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

// EnvPrefix is the prefix of every environment variable read as a Config
// Source: server.port is set by PW_SERVER_PORT.
const EnvPrefix = "PW"

// dirName is the directory under $HOME searched for a Config File.
const dirName = ".personal-web"

// DevDatabaseURL is the connection string a developer gets without configuring
// anything. It is duplicated as the fallback in database.yml, which is the one
// value ADR-0005 knowingly writes twice; keep the two identical. It is not a
// secret, and a deployment that reaches it fails loudly at the startup ping
// rather than serving from the wrong database.
const DevDatabaseURL = "postgres://postgres:postgres@127.0.0.1:5432/pw_dev?sslmode=disable"

// New returns a viper instance with the defaults and the environment Config
// Source in place. Callers bind their flags to it before calling Load.
//
// Every default lives here and nowhere else; flags are declared with zero
// values so that a default is never written twice.
func New() *viper.Viper {
	v := viper.New()

	v.SetDefault("server.port", 8080)
	v.SetDefault("server.read_timeout", 5*time.Second)
	v.SetDefault("server.write_timeout", 10*time.Second)
	v.SetDefault("server.shutdown_timeout", 10*time.Second)
	v.SetDefault("database.url", DevDatabaseURL)
	v.SetDefault("database.pool", 5)
	v.SetDefault("database.idle_pool", 2)
	v.SetDefault("database.conn_max_lifetime", 30*time.Minute)
	v.SetDefault("log_level", "info")

	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	return v
}

// Load reads the Config File, merges every Config Source, and returns the
// validated Effective Config.
//
// An explicit path that cannot be read is an error. Without one, a missing
// Config File is not: the application runs on environment variables and
// defaults, which is the normal case in a container.
func Load(v *viper.Viper, path string) (*Config, error) {
	if err := readConfigFile(v, path); err != nil {
		return nil, err
	}

	var cfg Config
	// ErrorUnused turns a typo in the Config File into a failure instead of a
	// value that is silently ignored.
	err := v.Unmarshal(&cfg, func(dc *mapstructure.DecoderConfig) {
		dc.ErrorUnused = true
	})
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	return &cfg, nil
}

func readConfigFile(v *viper.Viper, path string) error {
	if path != "" {
		v.SetConfigFile(path)
		if err := v.ReadInConfig(); err != nil {
			return fmt.Errorf("config: reading %s: %w", path, err)
		}
		return nil
	}

	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	if home, err := os.UserHomeDir(); err == nil {
		v.AddConfigPath(filepath.Join(home, dirName))
	}

	var notFound viper.ConfigFileNotFoundError
	if err := v.ReadInConfig(); err != nil && !errors.As(err, &notFound) {
		return fmt.Errorf("config: %w", err)
	}
	return nil
}
