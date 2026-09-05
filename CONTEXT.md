# go-personal-web

A personal website served by a single Go binary. The binary is a command-line
application: serving the site is one of its commands, alongside operational
commands for inspecting and diagnosing a deployment.

## Language

### Commands and configuration

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

### Content

**Post**:
A dated piece of writing that appears in the site's chronological index. Posts
are the material the index, the archive and the feed are built from.
_Avoid_: article, entry, blog post

**Page**:
A standalone piece of writing addressed only by its Slug and never listed
chronologically — `/about`, `/uses`. A Page is not a Post without a date; it is
a different thing that happens to share a shape.
_Avoid_: static page, document, content

**Slug**:
The URL-safe name that addresses a Post or Page publicly. It is the only
identifier a visitor ever sees.
_Avoid_: permalink, path, url, key

**Publication Date**:
The moment from which a Post or Page is public. Absent, the writing is a Draft;
set in the future, it is scheduled and appears on its own. There is no separate
status: this one date carries every state.
_Avoid_: status, published flag, state, visibility

**Draft**:
A Post or Page with no Publication Date. Invisible to the public and visible to
the Author, through the same URL that will serve it once published.
_Avoid_: unpublished, private, hidden

**Author**:
The single person who writes the site. There is no second one, and no notion of
registering, inviting or listing them.
_Avoid_: user, admin, account, owner
