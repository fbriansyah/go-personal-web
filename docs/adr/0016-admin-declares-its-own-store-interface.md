# Admin declares its own store interface

`internal/admin` defines a small interface naming exactly the reads and writes
its screens need, and `*content.Store` satisfies it without knowing. Handler,
session, rate-limit and rendering tests run against a stub, with no Docker, in
`make test-unit`.

This looks like a reversal of ADR-0004, which called a fake store "a fake
standing in for PostgreSQL [that] would test the half that does not break". It
is not. That decision refused to let `content` export an interface for itself,
and refused to test query semantics against a fake. Both hold: `content`
declares no interface, and `publishedClause`, Audience filtering and constraint
translation are still exercised against a real PostgreSQL in their own package.
The interface here belongs to the consumer, is declared where it is used, and
names what Admin needs rather than what the store offers.

What the stub tests is not the half that does not break. It is this list, none
of which touches SQL, and all of which are decisions made in this design:

- an htmx request with no session answers `401` with `HX-Redirect`, not `303`
- an ordinary request with no session answers `303` to the login form
- `HX-Request` present yields a fragment; absent yields a full page
- a duplicate slug answers `422` with the typed values still in the form
- the Nth login attempt answers `429` before argon2id runs
- autosave against published writing is refused

Making that list depend on Docker is making it rarely run.

## Consequences

- The interface doubles as a readable statement of the store surface Admin
  touches.
- Rendering tests assert on meaning — status, `HX-*` headers, the presence of
  particular values and attributes — never on whole-HTML golden files. A golden
  file would fail on every Tailwind class change until updating it without
  reading became the habit.
- `content`'s own tests keep needing a database, exactly as ADR-0004 intended.
