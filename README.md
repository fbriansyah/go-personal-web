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
| `migrate up` | Apply every pending migration |
| `migrate down --step N` | Roll back N migrations, newest first |
| `migrate status` | Show which migrations have been applied |
| `config show` | Print the effective configuration |
| `version` | Print the version and revision |

`/healthz` answers for the process alone, so a Postgres restart does not get a
healthy binary killed. `/readyz` answers for the process *and* its database.

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

Two containment rules hold the shape: viper does not leave `internal/config`,
and pop does not leave `internal/database` and `internal/content`. Everything
else receives ordinary Go types.

## Decisions

`CONTEXT.md` is the glossary — the words this project uses and the ones it
avoids. `docs/adr/` records the decisions that would otherwise look surprising,
including the several places where this project deliberately does not follow
pop's documented path.

## Not built yet

Public pages and the admin UI, along with the authentication described in
ADR-0009, land on a separate branch. What is here is the data layer and the
commands around it.
