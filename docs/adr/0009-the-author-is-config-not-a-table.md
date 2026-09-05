# The Author is a config value, not a database row

`/admin` authenticates against a single argon2id password hash supplied as a
Config Source (`PW_ADMIN_PASSWORD_HASH`), and issues an HMAC-signed session
cookie keyed by `PW_SESSION_SECRET`. There is no `users` table, no user model, no
registration, no password reset, and no session storage.

There is exactly one Author and there will not be a second. A `users` table for a
single row buys a seeding step, a password-change flow and a migration, and
answers a question nobody is going to ask. Keeping the credential in config also
keeps the promise CONTEXT.md opens with — a personal website served by a single
Go binary — where an external identity provider would not.

## Consequences

- Both secrets are held in the same redacting type as the DSN (ADR-0005), so
  neither can reach `config show`, a log line, or an error message.
- Rotating the password is an environment change and a restart, not a database
  write. Rotating `PW_SESSION_SECRET` invalidates the outstanding session, which
  is the intended way to log every device out.
- A second author would mean revisiting this decision rather than extending it.
  That is the intended cost.
