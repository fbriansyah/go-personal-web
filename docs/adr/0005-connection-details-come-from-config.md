# Connection details come from the Effective Config, not database.yml

Pop is connected with `pop.NewConnection(&pop.ConnectionDetails{...})`, filled
from the `database` block of the `*Config` struct. `pop.Connect`, the
`pop.Connections` global map, and `database.yml`-as-runtime-config are all
unused: the database is configured by the same flag > env > file > default
ranking as everything else, under the same `PW_` prefix, and `config show`
reports it.

`database.yml` still exists, but only as a template for the `soda` CLI in
development. Every entry in it is `{{ envOr "PW_DATABASE_URL" "<dev default>" }}`,
so the environment variable is the shared source of truth and the only value
written twice is a local development default that is not a secret. It never
ships to production, where migrations run from the binary (ADR-0007).

## Consequences

- The DSN is held in a type whose `MarshalYAML` and `String` always redact the
  password. Redaction lives in the type rather than in `config show`, so a DSN
  cannot leak through a log line or a `%v` added later. There is deliberately no
  `--show-secrets` escape hatch — it would end up in shell history and CI logs,
  which is exactly what the redaction is for.
- `soda migrate` must not be used, even though `database.yml` would let it work.
  It writes to the same `schema_migration` table as `personal-web migrate` but
  from a different set of files.
