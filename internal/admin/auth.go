package admin

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/fbriansyah/go-personal-web/internal/view"
)

// refused is what a failed login says, whatever the reason.
//
// A wrong password and a rate-limited attempt are answered identically:
// distinguishing them tells an attacker how close they are, and a
// "too many attempts" shown only after a correct guess would leak the guess
// (ADR-0015).
const refused = "That did not work."

// requireSession is the guard on everything but the login form and the assets.
func (h *handler) requireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, err := h.sessions.verify(r); err != nil {
			h.rejectUnauthenticated(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// rejectUnauthenticated sends the Author back to the login form, by the route
// that suits the request.
//
// An ordinary request is redirected. An htmx request is not: XHR follows a
// redirect transparently, so htmx would receive the login page as a 200 and
// swap it into whatever hx-target the failed action named — a login form
// embedded in a div, with the URL unchanged and no error anywhere. 401 with
// HX-Redirect makes htmx navigate for real (ADR-0011).
func (h *handler) rejectUnauthenticated(w http.ResponseWriter, r *http.Request) {
	to := "/admin/login"
	if next := returnPath(r); next != "" {
		to += "?next=" + url.QueryEscape(next)
	}

	if isHTMX(r) {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

// returnPath is the screen to come back to after logging in. Only a GET is
// worth returning to; replaying a POST after a login is not something the
// Author asked for.
func returnPath(r *http.Request) string {
	if r.Method != http.MethodGet {
		return ""
	}
	if r.URL.Path == "/admin/login" {
		return ""
	}
	return r.URL.RequestURI()
}

func (h *handler) loginForm(w http.ResponseWriter, r *http.Request) {
	f := view.LoginForm{Next: safeNext(r.URL.Query().Get("next"))}
	h.render(w, r, screen{title: "Log in", nav: view.NavNone, body: view.Login(f)})
}

func (h *handler) login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	next := safeNext(r.PostFormValue("next"))

	// The limit stands in front of the hash, not behind it: argon2id is
	// expensive by design, and behind the check that cost is already spent
	// (ADR-0015).
	if !h.logins.allow(clientIP(r)) {
		h.log.Warn("admin: login attempts refused", "ip", clientIP(r))
		// The status says what happened to the request; the body says exactly
		// what a wrong password says. Nothing here depends on whether the
		// password was right, because it was never checked.
		h.renderLoginFailure(w, r, next, http.StatusTooManyRequests)
		return
	}

	ok, err := verifyPassword(h.hash, r.PostFormValue("password"))
	if err != nil {
		// The configured hash is unusable. That is a deployment fault, not a
		// failed login, and saying so in the log is the only way it gets fixed.
		h.log.Error("admin: password hash is unusable", "err", err)
		h.renderLoginFailure(w, r, next, http.StatusUnauthorized)
		return
	}
	if !ok {
		h.renderLoginFailure(w, r, next, http.StatusUnauthorized)
		return
	}

	h.logins.reset(clientIP(r))
	http.SetCookie(w, h.sessions.issue())
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (h *handler) renderLoginFailure(w http.ResponseWriter, r *http.Request, next string, status int) {
	f := view.LoginForm{Next: next, Error: refused}
	h.render(w, r, screen{
		title:  "Log in",
		nav:    view.NavNone,
		body:   view.Login(f),
		status: status,
	})
}

// logout removes the cookie from this browser. It is all logging out can do:
// with no session storage there is nothing to revoke, and a copy taken
// elsewhere stays valid until it expires. Rotating the signing secret is the
// immediate revocation (ADR-0009).
func (h *handler) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, h.sessions.clear())
	http.Redirect(w, r, "/admin/login", http.StatusSeeOther)
}

// safeNext keeps a return path from becoming an open redirect. Only a path
// inside Admin survives; anything else falls back to the Admin index.
func safeNext(next string) string {
	if next == "" || !strings.HasPrefix(next, "/admin") || strings.HasPrefix(next, "//") {
		return "/admin/posts"
	}
	if _, err := url.ParseRequestURI(next); err != nil {
		return "/admin/posts"
	}
	return next
}
