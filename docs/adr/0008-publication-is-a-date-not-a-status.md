# Publication is a date, and Posts and Pages are separate

A Post or Page carries one `published_at` timestamp and no status column. NULL is
a Draft, a future value is scheduled, and a past value is live — so the public
filter is `published_at is not null and published_at <= now()` everywhere, served
by a partial index. A separate status column would be a second field capable of
contradicting the first, needing a `CHECK` to keep them honest.

Drafts are previewed through the public URL, with the Author's session relaxing
the filter rather than a second route rendering a second time. What you preview
is what will publish, because it is the same code path.

Posts and Pages are separate tables and separate models even though their columns
are identical today. The divergence that is actually coming — tags, series, a
feed, chronological pagination — belongs to Posts and not to Pages, and a shared
table would have paid for it with a discriminator column and queries that must
remember to filter on it.

## Consequences

- Bodies are stored as Markdown only and rendered on read. One source of truth
  means Markdown and HTML cannot drift, and upgrading the renderer or fixing the
  sanitizer applies to everything already written without a backfill.
- A scheduled Post appears without anything running. There is no scheduler,
  because `now()` is evaluated by the query.
- PostgreSQL's `now()` is the *transaction's* start time, not the statement's.
  That is the behaviour we want — every read serving one request agrees on one
  instant — but it has a consequence for tests: inside the transaction each test
  rolls back, the clock is frozen, so no amount of waiting makes a scheduled
  Post arrive. Tests assert the boundary with dates either side of it instead.
