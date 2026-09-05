# Config crosses the CLI boundary as a typed struct

Viper is confined to `internal/config`. It reads the Config Sources, and the
result is unmarshalled once into a `*Config` struct that is passed to whatever
needs it. No handler, server, or service ever calls `viper.GetString`.

The alternative — reading `v.GetInt("server.port")` at each call site — spreads
stringly-typed keys across the codebase, turns every typo into a runtime
surprise, and makes viper a dependency of code that has nothing to do with
configuration. A struct makes config an ordinary value that tests can construct
by hand.

## Consequences

- Adding a config key means adding a struct field; a key with no field is a
  config error, not a silently ignored one (see ADR-0003).
- Config is resolved once, at command start. Hot-reloading config at runtime is
  not supported and would require revisiting this decision.
