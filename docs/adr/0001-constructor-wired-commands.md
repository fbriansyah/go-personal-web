# Commands are wired by constructors, not package-level state

The cobra documentation and `cobra-cli init` produce a package-level `rootCmd`
that subcommands attach themselves to from `init()`, backed by viper's global
singleton. We deliberately do not do this: every command is built by a
`newXxxCmd(...) *cobra.Command` function and assembled explicitly in
`NewRootCmd`, and viper is a `*viper.Viper` instance created per invocation.

The reason is testability. Package-level commands and a global viper carry state
between tests in the same binary — a flag set in one test is still set in the
next, and there is no way to run two configurations in one process. Constructor
wiring costs a few lines of assembly and buys tests that can build a fresh
command tree, feed it `SetArgs`, and capture its output.

## Consequences

- No command calls `os.Exit` or `log.Fatal`; they return errors and `main` is the
  only place that decides an exit code. The root sets `SilenceErrors`, and
  silences usage from `PersistentPreRun` rather than via the `SilenceUsage`
  field — the field would also suppress the help text for a mistyped flag, which
  is precisely the case that deserves it. The hook runs only after flag and
  argument validation have passed, so usage survives for usage mistakes and
  disappears for runtime failures.
- The build-info values injected via `-ldflags` live in `main` and are passed
  into `NewRootCmd`, since `internal/cli` has no globals to inject into.
- A future reader arriving from a cobra tutorial will find the layout
  unfamiliar; that is intended.
