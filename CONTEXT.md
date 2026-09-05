# go-personal-web

A personal website served by a single Go binary. The binary is a command-line
application: serving the site is one of its commands, alongside operational
commands for inspecting and diagnosing a deployment.

## Language

**Command**:
A named action the binary can perform, invoked as `personal-web <command>`.
Serving the site (`serve`) is a command like any other, not a privileged default.
_Avoid_: subcommand, verb, action

**Effective Config**:
The single set of values the application actually runs with, after every
Config Source has been merged. It is what `config show` prints and what `serve`
obeys.
_Avoid_: merged config, final config, resolved settings

**Config Source**:
One of the four origins a config value may come from — flag, environment
variable, config file, or built-in default — ranked in that order of authority.
_Avoid_: layer, provider, backend

**Config File**:
An optional YAML file supplying config values. Its absence is normal, not an
error; its presence is a convenience for development.
_Avoid_: settings file, config, yaml

**Ops Command**:
A command that exists to inspect or maintain a deployment rather than serve it —
`version`, `config show`. They must remain safe to run against a live system.
_Avoid_: admin command, utility, tool
