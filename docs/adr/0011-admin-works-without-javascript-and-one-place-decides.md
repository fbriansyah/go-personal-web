# Admin works without JavaScript, and one place decides how a response is framed

Every action in Admin is a real `<form method=post>` or a real `<a href>`.
htmx intercepts some of them to swap a fragment, but nothing depends on it: with
scripting off, Admin still lists, writes, edits and publishes.

Handlers do not branch on this. A handler produces a component and its data, and
a single render function inspects `HX-Request` once — wrapping the component in
the Layout when the header is absent, writing it bare when it is present. The
promise costs one `if` in the application rather than one `if` per handler.

The reason is not visitors without JavaScript; there is exactly one visitor and
it is the Author. The reason is that every screen keeps a URL that can be opened
cold, which makes it reloadable when htmx misbehaves, bookmarkable, and — most
of all — testable with one `httptest` request and no browser.

Two consequences of this shape are easy to get wrong, so they are settled here.

**`hx-boost` is not used.** A boosted link also sends `HX-Request: true` while
meaning the opposite: it wants a whole page. The one branch this design rests on
would answer it with a fragment, and the screen would lose its layout with no
error anywhere. `HX-Boosted` could disambiguate, but that turns the most
load-bearing condition in the application into one with a forgettable second
half whose failure mode is a silently broken page. Boost earns its keep when
reloading a page means re-fetching heavy assets; here the assets are one
embedded stylesheet and one embedded htmx, cached, served by the same process.

**An expired session is answered by shape.** An ordinary request gets `303` to
`/admin/login?next=…`. An htmx request gets `401` with `HX-Redirect`, because
XHR follows a redirect transparently — htmx would receive the login page as a
`200` and swap it into whatever `hx-target` the failed action named.

## Consequences

- Validation failures answer `422` with the same form fragment, values intact.
  htmx does not swap error responses by default, so `htmx.config.responseHandling`
  admits `422`. Answering `200` for a failure was rejected: it would lie to the
  logs and to the tests.
- htmx is used only where it earns its place — autosave and search. Anything
  else is a plain navigation.
- The rules above are unit-testable without a database, and are tested
  (ADR-0016).
