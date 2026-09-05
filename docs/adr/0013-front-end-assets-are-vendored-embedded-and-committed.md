# Front-end assets are vendored, embedded, and their generated forms committed

htmx is a versioned file in the repository (`htmx-2.x.y.min.js`), embedded with
`go:embed` and served from `/admin/static`. No CDN. The stylesheet is Tailwind
output, also embedded. Both the generated CSS and the generated `*_templ.go`
files are committed, and `make check` fails if regenerating them changes
anything.

This is ADR-0007's argument applied again. That decision embedded migrations so
a deployment "carries its own schema and needs no files on disk and no `soda`".
A project that refuses to read migration files from disk, then fetches its
JavaScript from someone else's machine when a page loads, is not keeping the
promise `CONTEXT.md` opens with. The stakes are higher here than for
migrations: a third-party script on `/admin` runs on the page holding the
session cookie.

Owning the assets makes a strict Content-Security-Policy cheap, so it is set:
`default-src 'self'`, no `unsafe-inline`, no `unsafe-eval`, and
`htmx.config.allowEval` false. htmx's own configuration travels as a
`<meta name="htmx-config">` tag, which is not a script and needs no exception.
The policy is what rules out inline `<style>` and inline `hx-on:` handlers, and
therefore templ's `css` expressions (ADR-0010).

Tailwind was chosen over a hand-written stylesheet. It is the one decision here
that pulls in a tool the project did not need, and it is made tolerable by the
standalone CLI: Tailwind v4 ships a single binary per platform, so there is no
Node, no `package.json` and no `node_modules`. A Makefile target fetches a
pinned version into `bin/`, the way `make db-up` provides its own Postgres
rather than asking for one.

Committing generated output is what keeps `git clone && go build` working.
`go:embed` is a compile error when its file is missing, so an ignored stylesheet
would mean the repository does not build without two extra tools — exactly the
role `soda` was refused. Tools are for changing this project, not for building
it.

## Consequences

- The `make check` guard exists because stale generated files fail silently and
  misleadingly: a stale template renders old markup with no error, and a stale
  stylesheet drops styles for classes plainly written in the template. You would
  suspect htmx, or Tailwind, or the browser. `git diff --exit-code` turns the
  whole class into a failure that names itself.
- Tailwind scans files as plain text, so every class name in `internal/view`
  must appear as a complete literal. `"text-" + colour + "-600"` compiles,
  renders, and is never generated.
- Updating htmx is a commit that changes a filename, so `git log` says which
  version is deployed.
- `make check` now needs `templ` and `tailwindcss`; the Makefile fetches the
  latter and pins it.
