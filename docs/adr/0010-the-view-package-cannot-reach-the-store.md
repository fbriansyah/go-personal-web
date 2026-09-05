# The view package cannot reach the store

templ components live in `internal/view` and nothing else does. They receive
ordinary Go values — a `content.Post`, a slice of them, a form's current values
and errors — and they never import `content.Store`, never see a
`*http.Request`, and never perform a query. Handlers live in `internal/admin`,
fetch everything a screen needs, and call a component with it.
`internal/server` shrinks to what it is good at: listening, timeouts and
shutdown.

templ makes calling a store from inside a template both possible and pleasant,
and that is the failure this boundary exists to prevent. A query inside a
`for` loop in a template is invisible in review, untestable without a database,
and the usual cause of a page that issues forty queries. Making it a compile
error rather than a habit is cheaper than noticing it later.

The same package rule settles two smaller questions. Generated `*_templ.go`
files collect in one directory instead of interleaving with handler code. And
templ's `css` expressions — which emit a `<style>` block into the document — are
not used here at all: they collide with the Content-Security-Policy in
ADR-0013, and CSS belongs in the embedded stylesheet. That templ documents the
`css` feature as the normal path is precisely why this needs writing down.

## Consequences

- Giving a screen one more piece of data means editing two files: the
  component's signature and the handler that calls it. That friction is the
  point; it sits where the thinking belongs.
- `internal/view` compiles and renders with no database and no HTTP, so
  component tests are ordinary Go tests.
- When the public site arrives it gets its own handler package and shares
  `internal/view`. The shared Layout has an obvious home rather than becoming a
  question at that moment.
