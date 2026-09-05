package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func newConfigCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Inspect configuration",
		Args:  cobra.NoArgs,
		// Without a subcommand there is nothing to do; show the usage text.
		RunE: func(cmd *cobra.Command, _ []string) error { return cmd.Help() },
	}
	cmd.AddCommand(newConfigShowCmd(a))
	return cmd
}

// newConfigShowCmd prints the Effective Config. It accepts the same server
// flags as serve so that it answers "what would serve run with?" rather than
// "what would config show run with?".
func newConfigShowCmd(a *app) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Print the effective configuration",
		Long: "Print the configuration after flags, environment variables, the config file\n" +
			"and defaults have been merged. Values are shown merged, not attributed:\n" +
			"the source a value came from is not recorded.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := a.config(cmd)
			if err != nil {
				return err
			}

			out, err := yaml.Marshal(cfg)
			if err != nil {
				return err
			}

			if file := a.v.ConfigFileUsed(); file != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "# config file: %s\n", file)
			} else {
				fmt.Fprintln(cmd.OutOrStdout(), "# config file: none")
			}
			_, err = cmd.OutOrStdout().Write(out)
			return err
		},
	}
	addServerFlags(cmd)
	return cmd
}
