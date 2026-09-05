# Migrations are embedded SQL, run explicitly, and never by serve

Migrations are plain `.up.sql` / `.down.sql` files, embedded into the binary with
`//go:embed` and run through `pop.NewMigrationBox` by a `personal-web migrate`
command. Nothing else migrates: `soda` is not installed in any image, and `serve`
never applies a migration.

Fizz was rejected because its one real advantage is portability across dialects,
and Postgres is a fixed choice here. What is left is a DSL between the author and
DDL they already know, with Postgres-specific work — partial indexes, `CHECK`,
generated columns, GIN indexes for search — falling back to raw SQL anyway.

`serve` does check. After opening the connection and pinging, it counts pending
migrations and refuses to start if there are any, naming them and the command to
run. A binary serving a schema older than itself fails per-request with confusing
SQL errors; failing once, clearly, at startup is the same bargain already made
for `net.Listen` and for the ping itself. Deploying is `migrate up && serve`.

## Consequences

- Restarting a process never changes the schema, and rolling a binary back never
  leaves DDL behind that the rollback did not undo.
- Pop provides no "how many migrations are pending" call — `Status` only writes a
  table to an `io.Writer`. `Migrator.UpMigrations.Migrations` and
  `MigrationTableName()` are exported, so the count is computed directly rather
  than by parsing `Status` output.
- `migrate down` refuses to run while any migration is pending. Pop decides
  which migrations to reverse by counting rows in `schema_migration` and
  slicing the version-sorted list, rather than comparing versions against what
  is recorded — so a pending migration sorting before an applied one makes it
  reverse the wrong migration and report success. A fully applied schema is the
  only state where that arithmetic holds. This is also why migration timestamps
  must be UTC: a file stamped ahead of the clock puts the next one behind it.
- `Migrator.exec` takes no lock, so two concurrent `migrate up` runs race in the
  middle of DDL. `migrate up` wraps `Up()` in a Postgres advisory lock to close
  this; it is our code because pop has nowhere to put it.
- `Migrator.SchemaPath` is left empty. Set, it would make every migration end by
  shelling out to `pg_dump`, which is absent from the image. `NewMigrator`
  already leaves it empty, so this is a thing not to add rather than a thing to
  do — which is why it is recorded here instead of in a line of code.
- The pending check performs no DDL either: it does not call
  `CreateSchemaMigrations`, and reads a missing `schema_migration` table as
  "nothing applied yet". Otherwise `serve` would create a table on every start
  against a virgin database, which is exactly the rule this ADR sets.
