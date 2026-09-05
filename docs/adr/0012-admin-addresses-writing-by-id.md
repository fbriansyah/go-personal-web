# Admin addresses writing by ID, not by Slug

Admin URLs are `/admin/posts/{uuid}/edit`. The Slug addresses writing publicly
and nowhere else.

Admin is the only place a Slug ever changes, which makes it the one place a Slug
must not be an address. A form that posts back to `/admin/posts/{slug}` breaks
the moment the Author renames the slug it is editing, and under htmx it breaks
without the page reload that would normally hide the seam: the next autosave
posts to a slug that no longer exists and answers 404 into a fragment.

`CONTEXT.md` calls the Slug "the only identifier a visitor ever sees". The
Author is not a visitor, so the glossary never granted the Slug authority here.

## Consequences

- `PostByID` and `PageByID` join the store. Both take an `Audience` like every
  other read, so an Admin-only lookup cannot become a way to leak a Draft.
- Admin URLs are UUIDs and unpleasant to read. Nobody but the Author sees them,
  and ADR-0006 already bought time-ordered identifiers; this spends them.
- Slugs may be rewritten freely, which is now stated in `CONTEXT.md`. Whether an
  old Slug keeps working for visitors is a separate question, unanswered here.
