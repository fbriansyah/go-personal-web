# Drafts autosave; published writing does not

The Admin editor posts the Draft it is editing every few seconds. When the
writing has a Publication Date in the past, autosave is off and only an explicit
save changes anything.

Autosave exists because of ADR-0015: a session expires on an absolute deadline
and cannot be renewed, so sooner or later it lapses mid-sentence, and the
redirect to the login page takes the textarea with it. Autosave reduces the loss
to seconds — and, because it is a request, it reveals a dead session long before
the Author reaches for the save button.

Applying it to published writing would be a different thing entirely. Deleting a
paragraph in order to rewrite it would publish the gap within seconds, to a URL
people read. The tempting repair — let editing a published Post quietly return
it to Draft — is worse: fixing a typo would remove a page from the site and
discard its original Publication Date.

The honest fix is a second body: a working revision alongside the published one,
with the public always reading the published version. That is a new domain
concept with a column, a migration and a lifecycle, and it is a feature in its
own right rather than a detail of building the UI. It is deliberately deferred;
this decision is the special case it will subsume, not something it must undo.

## Consequences

- The rule is stated in `CONTEXT.md` under Draft: a Draft is saved continuously,
  published writing changes only when the Author asks.
- Nothing is lost by the asymmetry except a safety net where the stakes are
  highest. That is the accepted cost, and the reason to expect a working
  revision later.
- A new Post becomes a row on the first explicit save, not before: the schema
  requires a title and a well-formed slug, and until a row exists there is no ID
  to autosave against. The unprotected window is the first few sentences.
  Creating the row on the first valid autosave was rejected — it would mean an
  editor whose ID appears mid-session, an `hx-post` target that changes under
  the form, and `HX-Push-Url` to match. That is the htmx bug class worth
  designing out rather than handling.
