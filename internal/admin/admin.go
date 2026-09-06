// Package admin is the private half of the site, where the Author writes.
//
// Every action here is a real form or a real link: with scripting off, Admin
// still lists, writes, edits and publishes. htmx makes two of those faster —
// search and autosave — and nothing depends on it (ADR-0011).
//
// Handlers fetch what a screen needs and hand ordinary values to internal/view,
// which cannot reach the store itself (ADR-0010).
package admin

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/config"
	"github.com/fbriansyah/go-personal-web/internal/content"
)

// Store is what Admin needs from the content store, named here rather than
// exported by content (ADR-0016).
//
// It is a consumer-side interface: *content.Store satisfies it without knowing,
// query semantics are still tested against a real PostgreSQL in their own
// package, and the handler rules that have nothing to do with SQL — session
// handling, response framing, rate limiting, validation messages — become
// testable without Docker.
//
// Every read takes an Audience, so an Admin-only lookup cannot quietly become a
// way to serve a Draft to the public.
type Store interface {
	FindPosts(ctx context.Context, aud content.Audience, l content.Listing) ([]content.Post, error)
	FindPages(ctx context.Context, aud content.Audience, l content.Listing) ([]content.Page, error)
	PostByID(ctx context.Context, id uuid.UUID, aud content.Audience) (*content.Post, error)
	PageByID(ctx context.Context, id uuid.UUID, aud content.Audience) (*content.Page, error)
	CreatePost(ctx context.Context, p *content.Post) error
	UpdatePost(ctx context.Context, p *content.Post) error
	CreatePage(ctx context.Context, p *content.Page) error
	UpdatePage(ctx context.Context, p *content.Page) error
}

// perPage is how many rows a list screen shows before offering "load more".
//
// One row more than this is fetched, which is how the button knows there is
// another page without a count query.
const perPage = 50

// handler holds everything the screens share for one running server.
type handler struct {
	store    Store
	sessions *sessions
	logins   *limiter
	hash     string
	zone     *time.Location
	log      *slog.Logger
	now      func() time.Time
}

// New returns the Admin surface mounted under /admin.
//
// It is built by a constructor and assembled by the caller, like every command
// in this project (ADR-0001): there is no package-level router and no global
// state, so a test can build as many as it likes.
func New(store Store, cfg *config.Config, log *slog.Logger) http.Handler {
	h := &handler{
		store:    store,
		sessions: newSessions(cfg.Admin.SessionSecret.Raw(), cfg.Admin.SessionTTL, cfg.Admin.InsecureCookie),
		logins:   newLimiter(cfg.Admin.LoginAttempts, cfg.Admin.LoginWindow),
		hash:     cfg.Admin.PasswordHash.Raw(),
		zone:     cfg.Admin.Location(),
		log:      log,
		now:      time.Now,
	}
	return h.routes()
}

func (h *handler) routes() http.Handler {
	mux := http.NewServeMux()

	// Served without a session, and the only paths that are.
	mux.HandleFunc("GET /admin/login", h.loginForm)
	mux.HandleFunc("POST /admin/login", h.login)
	mux.Handle("GET /admin/static/", http.HandlerFunc(serveAsset))

	// Everything else requires one.
	guarded := http.NewServeMux()
	guarded.HandleFunc("POST /admin/logout", h.logout)
	guarded.HandleFunc("GET /admin/{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/admin/posts", http.StatusSeeOther)
	})

	guarded.HandleFunc("GET /admin/posts", h.listPosts)
	guarded.HandleFunc("GET /admin/posts/new", h.newPost)
	guarded.HandleFunc("POST /admin/posts", h.createPost)
	guarded.HandleFunc("GET /admin/posts/{id}/edit", h.editPost)
	guarded.HandleFunc("POST /admin/posts/{id}", h.updatePost)
	guarded.HandleFunc("POST /admin/posts/{id}/autosave", h.autosavePost)

	guarded.HandleFunc("GET /admin/pages", h.listPages)
	guarded.HandleFunc("GET /admin/pages/new", h.newPage)
	guarded.HandleFunc("POST /admin/pages", h.createPage)
	guarded.HandleFunc("GET /admin/pages/{id}/edit", h.editPage)
	guarded.HandleFunc("POST /admin/pages/{id}", h.updatePage)
	guarded.HandleFunc("POST /admin/pages/{id}/autosave", h.autosavePage)

	mux.Handle("/admin/", h.requireSession(guarded))
	return secured(mux)
}
