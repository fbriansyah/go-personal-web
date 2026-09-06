package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/fbriansyah/go-personal-web/internal/admin"
)

// newAdminCmd groups the Ops Commands that exist because the Author is a config
// value rather than a database row (ADR-0009). Without hash-password there is
// no way to produce the one credential Admin checks against.
func newAdminCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "admin",
		Short: "Manage the Admin surface",
		Args:  cobra.NoArgs,
	}
	cmd.AddCommand(newHashPasswordCmd())
	return cmd
}

func newHashPasswordCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "hash-password",
		Short: "Print an argon2id hash for PW_ADMIN_PASSWORD_HASH",
		Long: "Read a password and print its argon2id hash.\n\n" +
			"The hash is the value of admin.password_hash (PW_ADMIN_PASSWORD_HASH).\n" +
			"Rotating the password is an environment change and a restart, not a\n" +
			"database write.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			password, err := readPassword(cmd)
			if err != nil {
				return err
			}
			if password == "" {
				return fmt.Errorf("no password given")
			}
			hash, err := admin.HashPassword(password)
			if err != nil {
				return err
			}
			// The hash goes to stdout alone, so it can be piped, while the
			// prompt above went to stderr.
			fmt.Fprintln(cmd.OutOrStdout(), hash)
			return nil
		},
	}
}

// readPassword takes the password from a terminal without echoing it, and from
// a pipe when there is no terminal — so the command works in a script without
// the password ever appearing in shell history as an argument.
func readPassword(cmd *cobra.Command) (string, error) {
	if term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprint(cmd.ErrOrStderr(), "Password: ")
		raw, err := term.ReadPassword(int(os.Stdin.Fd()))
		fmt.Fprintln(cmd.ErrOrStderr())
		if err != nil {
			return "", fmt.Errorf("reading password: %w", err)
		}
		return string(raw), nil
	}

	line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
	if err != nil && line == "" {
		return "", fmt.Errorf("reading password: %w", err)
	}
	return strings.TrimRight(line, "\r\n"), nil
}
