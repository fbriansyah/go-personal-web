// Package cli assembles the command tree. Every command is built by a
// constructor and wired together here; there is no package-level command and no
// global viper, so a test can build a fresh tree per case.
package cli

import (
	"log/slog"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/fbriansyah/go-personal-web/internal/config"
)

// app is the state shared by the commands of a single invocation.
type app struct {
	v       *viper.Viper
	cfgFile string
}

// NewRootCmd builds the command tree. Nothing runs at import time, so callers
// may build as many trees as they like.
func NewRootCmd(bi BuildInfo) *cobra.Command {
	a := &app{v: config.New()}

	root := &cobra.Command{
		Use:   "personal-web",
		Short: "Personal website server",
		// Errors are printed by main, in one format.
		SilenceErrors: true,
		// Usage is silenced only once parsing has succeeded: a bad flag or a
		// stray argument is a usage mistake and deserves the help text, while a
		// command that fails at runtime does not. This hook runs after flag and
		// argument validation, so it draws exactly that line. A subcommand
		// defining its own PersistentPreRun would replace it.
		PersistentPreRun: func(cmd *cobra.Command, _ []string) {
			cmd.SilenceUsage = true
		},
	}

	root.PersistentFlags().StringVar(&a.cfgFile, "config", "",
		"config file (default ./config.yaml, then $HOME/.personal-web/config.yaml)")
	root.PersistentFlags().String("log-level", "", "log level: debug|info|warn|error (default info)")

	root.AddCommand(
		newServeCmd(a),
		newConfigCmd(a),
		newVersionCmd(bi),
	)
	return root
}

// flagBindings maps a config key to the flag that overrides it. Binding is
// explicit so that flags stay short (--port) while keys stay structured
// (server.port).
var flagBindings = map[string]string{
	"log_level":           "log-level",
	"server.port":         "port",
	"server.read_timeout": "read-timeout",
}

// addServerFlags registers the flags that override server settings. Both serve
// and `config show` use it, so `config show --port 5000` is a dry run of what
// serve would see.
func addServerFlags(cmd *cobra.Command) {
	cmd.Flags().Int("port", 0, "HTTP listen port (default 8080)")
	cmd.Flags().Duration("read-timeout", 0, "HTTP read timeout (default 5s)")
}

// config resolves the Effective Config for cmd. Flags are bound first: an
// unchanged flag ranks below every other Config Source, so declaring one costs
// nothing until the user sets it.
func (a *app) config(cmd *cobra.Command) (*config.Config, error) {
	for key, name := range flagBindings {
		f := cmd.Flags().Lookup(name)
		if f == nil {
			continue
		}
		if err := a.v.BindPFlag(key, f); err != nil {
			return nil, err
		}
	}
	return config.Load(a.v, a.cfgFile)
}

// newLogger builds the logger for cmd. Writing to the command's error stream
// rather than os.Stderr keeps the output capturable in tests.
func newLogger(cmd *cobra.Command, cfg *config.Config) *slog.Logger {
	return slog.New(slog.NewTextHandler(cmd.ErrOrStderr(), &slog.HandlerOptions{
		Level: cfg.SlogLevel(),
	}))
}
