package admin

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/a-h/templ"
	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/content"
	"github.com/fbriansyah/go-personal-web/internal/view"
)

// editorTimeFormat is the value shape of <input type="datetime-local">.
const editorTimeFormat = "2006-01-02T15:04"

// record is the part of a Post and a Page that the editor touches. The two
// screens are otherwise identical, and this is what lets them share one set of
// handlers without either type learning about the other.
type record struct {
	ID          uuid.UUID
	Slug        string
	Title       string
	Body        string
	PublishedAt *time.Time
}

// resource is the half of the editor that differs between a Post and a Page.
type resource struct {
	kind string
	nav  view.Nav

	byID   func(ctx context.Context, id uuid.UUID) (*record, error)
	create func(ctx context.Context, r *record) error
	// update writes the fields of r onto the stored row. Whether the
	// Publication Date is among them is the caller's decision, which is what
	// keeps autosave from ever publishing anything.
	update func(ctx context.Context, r *record, withDate bool) error
}

func (h *handler) posts() resource {
	return resource{
		kind: "post",
		nav:  view.NavPosts,
		byID: func(ctx context.Context, id uuid.UUID) (*record, error) {
			p, err := h.store.PostByID(ctx, id, content.Author)
			if err != nil {
				return nil, err
			}
			return &record{ID: p.ID, Slug: p.Slug, Title: p.Title, Body: p.Body, PublishedAt: p.PublishedAt}, nil
		},
		create: func(ctx context.Context, r *record) error {
			p := &content.Post{Slug: r.Slug, Title: r.Title, Body: r.Body, PublishedAt: r.PublishedAt}
			if err := h.store.CreatePost(ctx, p); err != nil {
				return err
			}
			r.ID = p.ID
			return nil
		},
		update: func(ctx context.Context, r *record, withDate bool) error {
			p, err := h.store.PostByID(ctx, r.ID, content.Author)
			if err != nil {
				return err
			}
			p.Slug, p.Title, p.Body = r.Slug, r.Title, r.Body
			if withDate {
				p.PublishedAt = r.PublishedAt
			}
			return h.store.UpdatePost(ctx, p)
		},
	}
}

func (h *handler) pages() resource {
	return resource{
		kind: "page",
		nav:  view.NavPages,
		byID: func(ctx context.Context, id uuid.UUID) (*record, error) {
			p, err := h.store.PageByID(ctx, id, content.Author)
			if err != nil {
				return nil, err
			}
			return &record{ID: p.ID, Slug: p.Slug, Title: p.Title, Body: p.Body, PublishedAt: p.PublishedAt}, nil
		},
		create: func(ctx context.Context, r *record) error {
			p := &content.Page{Slug: r.Slug, Title: r.Title, Body: r.Body, PublishedAt: r.PublishedAt}
			if err := h.store.CreatePage(ctx, p); err != nil {
				return err
			}
			r.ID = p.ID
			return nil
		},
		update: func(ctx context.Context, r *record, withDate bool) error {
			p, err := h.store.PageByID(ctx, r.ID, content.Author)
			if err != nil {
				return err
			}
			p.Slug, p.Title, p.Body = r.Slug, r.Title, r.Body
			if withDate {
				p.PublishedAt = r.PublishedAt
			}
			return h.store.UpdatePage(ctx, p)
		},
	}
}

func (h *handler) newPost(w http.ResponseWriter, r *http.Request)  { h.blank(w, r, h.posts()) }
func (h *handler) newPage(w http.ResponseWriter, r *http.Request)  { h.blank(w, r, h.pages()) }
func (h *handler) editPost(w http.ResponseWriter, r *http.Request) { h.edit(w, r, h.posts()) }
func (h *handler) editPage(w http.ResponseWriter, r *http.Request) { h.edit(w, r, h.pages()) }

func (h *handler) createPost(w http.ResponseWriter, r *http.Request) { h.save(w, r, h.posts(), true) }
func (h *handler) createPage(w http.ResponseWriter, r *http.Request) { h.save(w, r, h.pages(), true) }
func (h *handler) updatePost(w http.ResponseWriter, r *http.Request) { h.save(w, r, h.posts(), false) }
func (h *handler) updatePage(w http.ResponseWriter, r *http.Request) { h.save(w, r, h.pages(), false) }

func (h *handler) autosavePost(w http.ResponseWriter, r *http.Request) { h.autosave(w, r, h.posts()) }
func (h *handler) autosavePage(w http.ResponseWriter, r *http.Request) { h.autosave(w, r, h.pages()) }

// blank is the screen for writing that has no row yet.
//
// It stays that way until the first explicit save: the schema requires a title
// and a well-formed slug, so there is nothing to store before then, and until a
// row exists there is no ID to autosave against (ADR-0014).
func (h *handler) blank(w http.ResponseWriter, r *http.Request, res resource) {
	f := view.Form{Kind: res.kind, Zone: h.zone.String()}
	h.render(w, r, screen{title: "New " + res.kind, nav: res.nav, body: view.Editor(f)})
}

func (h *handler) edit(w http.ResponseWriter, r *http.Request, res resource) {
	rec, ok := h.load(w, r, res)
	if !ok {
		return
	}
	h.render(w, r, screen{
		title: rec.Title,
		nav:   res.nav,
		body:  view.Editor(h.formOf(res, rec)),
	})
}

// save handles both the first save and every one after it.
//
// A rejected save answers 422 with the form as the Author left it, not as the
// database holds it, so nothing typed is lost to a message about a slug
// (ADR-0011).
func (h *handler) save(w http.ResponseWriter, r *http.Request, res resource, isNew bool) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	f, rec, err := h.submitted(r, res, isNew)
	if err != nil {
		h.invalid(w, r, res, f, map[string]string{"published_at": err.Error()})
		return
	}

	// "Publish now" fills in the Publication Date rather than setting a flag:
	// the date is the only state there is (ADR-0008), and doing it on the
	// server means the button needs no JavaScript.
	if r.PostFormValue("action") == "publish_now" {
		now := h.now()
		rec.PublishedAt = &now
		f.PublishedAt = now.In(h.zone).Format(editorTimeFormat)
	}

	if isNew {
		err = res.create(r.Context(), rec)
	} else {
		err = res.update(r.Context(), rec, true)
	}
	if err != nil {
		if fields := fieldErrors(err); fields != nil {
			h.invalid(w, r, res, f, fields)
			return
		}
		if errors.Is(err, content.ErrNotFound) {
			h.notFound(w)
			return
		}
		h.fail(w, r, err)
		return
	}

	// Post-redirect-Get, for htmx as much as for the browser: the editor's URL
	// is the ID, and after the first save that ID exists.
	to := "/admin/" + res.kind + "s/" + rec.ID.String() + "/edit"
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// autosave stores a Draft as it is written.
//
// It refuses published writing: deleting a paragraph in order to rewrite it
// would otherwise put the gap in front of whoever is reading, seconds later
// (ADR-0014). The refusal is a 200 carrying a marker with no trigger on it, so
// the browser stops asking rather than looping against a status htmx will not
// swap. Nothing is written, which is what "refused" means here.
//
// It also never writes the Publication Date, even for a Draft. Publishing is an
// act the Author performs, and a date typed but not yet saved must not become
// live because fifteen seconds passed.
func (h *handler) autosave(w http.ResponseWriter, r *http.Request, res resource) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	stored, ok := h.load(w, r, res)
	if !ok {
		return
	}

	f := h.formOf(res, stored)
	if stored.PublishedAt != nil && !stored.PublishedAt.After(h.now()) {
		h.renderMarker(w, r, view.PublishedMarker())
		return
	}

	submitted, _, err := h.submitted(r, res, false)
	_ = err // a malformed date cannot reach the store: autosave never writes one.
	stored.Slug, stored.Title, stored.Body = submitted.Slug, submitted.Title, submitted.Body

	if err := res.update(r.Context(), stored, false); err != nil {
		if fields := fieldErrors(err); fields != nil {
			h.renderMarker(w, r, view.NotSavedMarker(f, firstOf(fields)))
			return
		}
		h.fail(w, r, err)
		return
	}
	h.renderMarker(w, r, view.Saved(f, h.now().In(h.zone).Format("15:04:05")))
}

func (h *handler) renderMarker(w http.ResponseWriter, r *http.Request, c templ.Component) {
	h.render(w, r, screen{nav: view.NavNone, body: c, fragment: c})
}

// invalid answers a rejected save.
func (h *handler) invalid(w http.ResponseWriter, r *http.Request, res resource, f view.Form, fields map[string]string) {
	f.Errors = fields
	h.render(w, r, screen{
		title:    "Edit " + res.kind,
		nav:      res.nav,
		body:     view.Editor(f),
		fragment: view.EditorForm(f),
		status:   http.StatusUnprocessableEntity,
	})
}

// load reads the writing named by the {id} path value, or answers for it.
func (h *handler) load(w http.ResponseWriter, r *http.Request, res resource) (*record, bool) {
	id, err := uuid.FromString(r.PathValue("id"))
	if err != nil {
		h.notFound(w)
		return nil, false
	}
	rec, err := res.byID(r.Context(), id)
	if err != nil {
		if errors.Is(err, content.ErrNotFound) {
			h.notFound(w)
			return nil, false
		}
		h.fail(w, r, err)
		return nil, false
	}
	return rec, true
}

// submitted reads the form the Author posted. The returned view.Form carries
// exactly what was typed, so a rejection can be shown against it.
func (h *handler) submitted(r *http.Request, res resource, isNew bool) (view.Form, *record, error) {
	f := view.Form{
		Kind:        res.kind,
		Slug:        r.PostFormValue("slug"),
		Title:       r.PostFormValue("title"),
		Body:        r.PostFormValue("body"),
		PublishedAt: r.PostFormValue("published_at"),
		Zone:        h.zone.String(),
	}
	if !isNew {
		f.ID = r.PathValue("id")
	}

	rec := &record{Slug: f.Slug, Title: f.Title, Body: f.Body}
	if !isNew {
		id, err := uuid.FromString(f.ID)
		if err != nil {
			return f, rec, errors.New("that is not a valid id")
		}
		rec.ID = id
	}

	published, err := h.parsePublishedAt(f.PublishedAt)
	if err != nil {
		return f, rec, err
	}
	rec.PublishedAt = published
	f.Autosave = published == nil || published.After(h.now())
	return f, rec, nil
}

// parsePublishedAt reads the datetime-local value in the Author's zone.
//
// The input carries no zone of its own, so one has to be chosen. It is the zone
// named in config, printed next to the field, rather than the server's: moving
// a deployment must not shift what a schedule means.
func (h *handler) parsePublishedAt(value string) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	t, err := time.ParseInLocation(editorTimeFormat, value, h.zone)
	if err != nil {
		return nil, errors.New("that is not a date this form understands")
	}
	return &t, nil
}

// formOf renders a stored record as the form that edits it.
func (h *handler) formOf(res resource, rec *record) view.Form {
	f := view.Form{
		ID:       rec.ID.String(),
		Kind:     res.kind,
		Slug:     rec.Slug,
		Title:    rec.Title,
		Body:     rec.Body,
		Zone:     h.zone.String(),
		Autosave: rec.PublishedAt == nil || rec.PublishedAt.After(h.now()),
	}
	if rec.PublishedAt != nil {
		f.PublishedAt = rec.PublishedAt.In(h.zone).Format(editorTimeFormat)
	}
	return f
}

// fieldErrors names the field a refused write belongs to, or nil when the error
// is not one the Author can act on.
//
// The schema is where the rules live, and content translates their enforcement
// into domain errors; this is the last step, from a domain error to the field
// it is shown against.
func fieldErrors(err error) map[string]string {
	switch {
	case errors.Is(err, content.ErrSlugTaken):
		return map[string]string{"slug": "Something else already answers to that slug."}
	case errors.Is(err, content.ErrSlugInvalid):
		return map[string]string{"slug": "Lowercase words joined by hyphens, nothing else."}
	case errors.Is(err, content.ErrTitleRequired):
		return map[string]string{"title": "A title is required."}
	}
	return nil
}

func firstOf(fields map[string]string) string {
	for _, v := range fields {
		return v
	}
	return "not saved"
}
