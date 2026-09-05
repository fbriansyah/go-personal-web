# Config resolution is strict about typos and lenient about missing files

Config Sources are ranked flag > environment variable > Config File > default.
Environment variables use the `PW_` prefix with `.` replaced by `_`
(`PW_SERVER_PORT`). Defaults live in exactly one place — `v.SetDefault` — and
flags are declared with zero values so that the default is not written twice.

Two asymmetric strictness choices:

- **A missing Config File is fine.** Without `--config`, the binary looks for
  `./config.yaml` then `$HOME/.personal-web/config.yaml` and proceeds on defaults
  and environment variables if neither exists. A container deployment should not
  need a dummy file. But an explicit `--config <path>` that does not exist is an
  error, because the user clearly expected it to be read.
- **An unknown key in the Config File is an error.** Unmarshalling uses
  `ErrorUnused`, so `prot: 8080` fails loudly instead of leaving the server on
  its default port. Config that is silently ignored is worse than config that is
  rejected.

## Consequences

- `config show` prints the Effective Config as YAML plus the Config File it used.
  It reports merged values only — viper does not retain which source won a given
  key, and reconstructing that provenance by hand was rejected as a second source
  of truth to keep in sync.
- The server flags (`--port`, `--read-timeout`) are registered by a shared helper
  used by **both** `serve` and `config show`, so `config show --port 5000` is a
  genuine dry run of what `serve` would see. This is why an Ops Command accepts
  flags that look like they belong to the server, and it lets precedence be
  tested end-to-end through cobra without opening a socket.
