# Pop is confined to internal/content

Pop reaches exactly one package. `internal/content` owns `*pop.Connection` and
exposes a concrete `Store` whose methods speak the domain — `BySlug`,
`Published` — so no handler, route or template ever sees a pop type. This is
ADR-0002's rule about viper applied to the second library with the same
appetite for spreading.

The idiomatic pop and Buffalo path is the opposite: hand `*pop.Connection` to
handlers and call `c.All(&posts)` where you need it. That is less code today and
makes pop a dependency of the whole application forever, including every test of
every handler.

## Consequences

- `Store` is a concrete struct, not an interface. One implementation behind an
  interface is a speculative abstraction, and a fake that never touches SQL
  would test the part that does not break. The price is real and was accepted
  knowingly: `internal/content` cannot be tested without a database. Its tests
  therefore start a Postgres container (`testcontainers-go`) once per package,
  migrate it with the embedded migrator, and isolate each case in a transaction
  that is rolled back. The suite stays runnable anywhere, as ADR-0003 assumes,
  at the cost of requiring Docker and a few seconds of startup — and every run
  now exercises the migrator as a side effect.
- Swapping pop for `sqlc`, `sqlx` or hand-rolled SQL is an edit to one package.
  This is the main thing the seam buys, and it is worth stating plainly because
  pop's own documentation argues against it.
