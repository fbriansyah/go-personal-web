# go-personal-web

A personal website served by a single Go binary. Serving the site is one command
among several; the others exist to inspect and maintain a deployment.

Content — Posts and Pages — lives in PostgreSQL, reached through
[pop](https://github.com/gobuffalo/pop).

## Requirements

- Go 1.26
- PostgreSQL 17 (the Makefile can run one in Docker)
- Docker, for the tests that need a database

## Getting started

```sh
make db-up        # postgres in docker, on :5432
make migrate      # apply the embedded migrations
make run          # serve on :8080
```

`make db-up` fails if something already holds :5432 — very likely your own
postgres. Either use that one, or move ours and tell the binary where it went:

```sh
make db-up DB_PORT=5433
export PW_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:5433/pw_dev?sslmode=disable'
```

`make help` lists every target.

## Configuration

Values come from four sources, ranked **flag > environment variable > config
file > default**. Environment variables use the `PW_` prefix with `.` replaced
by `_`, so `server.port` is `PW_SERVER_PORT` and `database.url` is
`PW_DATABASE_URL`.

A missing config file is normal; the binary runs on defaults and environment
variables, which is the usual case in a container. An explicit `--config path`
that cannot be read is an error, and an unknown key in a config file is an
error too — config that is silently ignored is worse than config that is
rejected. See `config.example.yaml` for every key.

```sh
make config      # or: personal-web config show
```

`config show` prints the merged result, and always redacts the database
password. That redaction lives in the DSN type rather than in the command, so
it applies to log lines and error messages as well.

## Commands

| Command | Purpose |
| --- | --- |
| `serve` | Run the HTTP server |
| `admin hash-password` | Print an argon2id hash for the Author's password |
| `migrate up` | Apply every pending migration |
| `migrate down --step N` | Roll back N migrations, newest first |
| `migrate status` | Show which migrations have been applied |
| `config show` | Print the effective configuration |
| `version` | Print the version and revision |

`/healthz` answers for the process alone, so a Postgres restart does not get a
healthy binary killed. `/readyz` answers for the process *and* its database.

## Admin

`/admin` is where the Author writes. It is mounted only when both secrets are
set; without them the binary serves the public site and says so at startup.

```sh
export PW_ADMIN_PASSWORD_HASH="$(make -s hash-password)"
export PW_ADMIN_SESSION_SECRET="$(make -s session-secret)"
export PW_ADMIN_INSECURE_COOKIE=true   # development only: allows plain HTTP
make run
```

There is no `users` table and no registration: the Author is a config value
(ADR-0009). Rotating the password is an environment change and a restart;
rotating `PW_ADMIN_SESSION_SECRET` logs every device out immediately.

A session lasts seven days from the moment it is issued and is never renewed,
because with nothing stored there is nothing to revoke against. The cookie is
`SameSite=Strict`, which is the whole of the CSRF defence, and `Secure` unless
`admin.insecure_cookie` says otherwise — a key you set, never a guess about
localhost.

Admin works with scripting off: every action is a real form or a real link.
htmx makes two of them better — search filters as you type, and a Draft saves
itself every fifteen seconds. Published writing does **not** autosave: deleting
a paragraph in order to rewrite it would otherwise put the gap in front of
whoever is reading (ADR-0014).

There is no publish button and no status column. The Publication Date is a
field: empty is a Draft, a past time is live, a future one is scheduled, and
"Publish now" simply fills it in. Times are read and shown in
`admin.timezone`, printed beside the field so it is never a guess.

## The front end

Components are [templ](https://templ.guide); the behaviour is
[htmx](https://htmx.org). Both htmx and the stylesheet are files in this
repository, embedded in the binary and served from `/admin/static` — no CDN, so
a strict Content-Security-Policy costs nothing and nobody else's JavaScript
runs on the page holding the session cookie.

The generated files — `internal/view/*_templ.go` and
`internal/admin/static/app.css` — are committed, so `git clone && go build`
works with no tools at all. `templ` and Tailwind are needed only to *change*
them, and the Makefile fetches both, pinned, into `bin/`:

```sh
make generate   # after editing a .templ file or a class name
make watch      # regenerate and reload while editing
```

`make check` fails if the committed generated files are stale. They fail
silently otherwise: a stale template renders old markup with no error, and a
stale stylesheet drops styles for classes plainly written in the template.

Tailwind reads the sources as plain text, so every class name must appear
whole. `"text-" + colour + "-600"` compiles, renders, and is never generated.

## Migrations

Migrations are plain SQL in `internal/database/migrations`, embedded into the
binary with `go:embed`. A deployment therefore carries its own schema and needs
no files on disk and no `soda`.

```sh
make migration name=add_tags   # scaffolds the up/down pair
```

`serve` never migrates. It refuses to start while any migration is pending,
naming what is missing, so deploying is:

```sh
personal-web migrate up && personal-web serve
```

Two rules worth knowing, both forced by how pop works:

- **Timestamps must be UTC.** `make migration` uses `date -u`. A file stamped
  ahead of the clock will sort after migrations written later.
- **`migrate down` refuses to run while anything is pending.** Pop picks the
  migrations to reverse by position in a sorted list rather than by version, so
  a pending migration out of order makes it reverse the wrong one — silently.
  Apply or remove pending migrations first.

`database.yml` exists only for the `soda` CLI in development. The application
never reads it. Do not run `soda migrate`: it writes to the same
`schema_migration` table from a different set of files.

## Tests

```sh
make test        # everything; content tests start postgres via testcontainers
make test-unit   # only the tests that need neither docker nor a database
make check       # fmt, vet, test
```

The content tests start their own PostgreSQL container, apply the embedded
migrations to it, and roll back a transaction per case. `make test` therefore
needs Docker but no setup, and every run exercises the migrator as a side
effect.

## Layout

```
internal/cli/        the command tree; every command built by a constructor
internal/config/     every config source, and the Effective Config they produce
internal/database/   the pop connection and the embedded migrations
internal/content/    Posts, Pages, and the store they are read from
internal/server/     the HTTP server
```

Three containment rules hold the shape: viper does not leave `internal/config`,
pop does not leave `internal/database` and `internal/content`, and
`internal/view` reaches neither the store nor the request. Everything else
receives ordinary Go types.

## Decisions

`CONTEXT.md` is the glossary — the words this project uses and the ones it
avoids. `docs/adr/` records the decisions that would otherwise look surprising,
including the several places where this project deliberately does not follow
pop's documented path.

## Not built yet

The public site. Posts and Pages can be written, but nothing serves them to a
visitor yet, and the Markdown in a body is stored rather than rendered — the
renderer is the public site's decision (ADR-0008).

Also deliberately absent: deleting writing (clearing the Publication Date
withdraws it), a live Markdown preview, and the working revision that would let
published writing autosave the way a Draft does (ADR-0014).
