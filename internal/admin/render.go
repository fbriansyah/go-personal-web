package admin

import (
	"net/http"

	"github.com/a-h/templ"

	"github.com/fbriansyah/go-personal-web/internal/view"
)

// screen is what a handler produces: a component, and how to frame it.
//
// Handlers never inspect the request to decide between a full page and a
// fragment. They describe the screen; render below makes that decision once,
// for the whole application (ADR-0011).
type screen struct {
	title string
	nav   view.Nav
	// body is wrapped in the Layout when the request is an ordinary one.
	body templ.Component
	// fragment is written bare to an htmx request. When it is nil the body is
	// used, which is the common case: an editor or a login form is the same
	// markup either way.
	fragment templ.Component
	// status defaults to 200.
	status int
}

// render writes a screen, deciding here and nowhere else whether it is framed.
//
// This one condition is what buys "Admin works without JavaScript" for the
// price of a single `if` rather than one per handler. hx-boost is deliberately
// not used anywhere, because a boosted link also sends HX-Request while meaning
// the opposite, and this branch would answer it with a fragment and silently
// strip the page of its layout (ADR-0011).
func (h *handler) render(w http.ResponseWriter, r *http.Request, s screen) {
	component := s.body
	if isHTMX(r) {
		if s.fragment != nil {
			component = s.fragment
		}
	} else {
		component = view.Layout(s.title, s.nav, s.body)
	}

	status := s.status
	if status == 0 {
		status = http.StatusOK
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Nothing in Admin may be cached: a back button that re-displays a screen
	// from a session that has since ended is a screen that shows a Draft to
	// whoever is now at the keyboard.
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	if err := component.Render(r.Context(), w); err != nil {
		// The status line is already written, so nothing useful can be said to
		// the browser. Say it to the log instead.
		h.log.Error("rendering", "path", r.URL.Path, "err", err)
	}
}

// isHTMX reports whether the response should be a bare fragment.
func isHTMX(r *http.Request) bool {
	return r.Header.Get("HX-Request") == "true"
}

// fail reports something that is nobody's fault but ours.
func (h *handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("admin", "path", r.URL.Path, "err", err)
	http.Error(w, "something went wrong", http.StatusInternalServerError)
}

// notFound answers for an ID that names nothing the Author may see.
func (h *handler) notFound(w http.ResponseWriter) {
	http.Error(w, "not found", http.StatusNotFound)
}
