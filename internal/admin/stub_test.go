package admin_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/admin"
	"github.com/fbriansyah/go-personal-web/internal/config"
	"github.com/fbriansyah/go-personal-web/internal/content"
)

// stub stands in for the store.
//
// It is not standing in for PostgreSQL — query semantics are tested against a
// real one in internal/content (ADR-0004). What it makes testable is the list
// in ADR-0016: session handling, response framing, rate limiting and validation
// messages, none of which touch SQL and all of which are decisions this design
// made deliberately.
type stub struct {
	posts []content.Post
	pages []content.Page

	// err is returned by the next write, which is how a constraint violation
	// is played back without a database.
	err error
	// writes counts stores that actually happened, so a test can assert that
	// something was refused rather than merely answered.
	writes int
}

func (s *stub) FindPosts(_ context.Context, _ content.Audience, l content.Listing) ([]content.Post, error) {
	return page(s.posts, l), nil
}

func (s *stub) FindPages(_ context.Context, _ content.Audience, l content.Listing) ([]content.Page, error) {
	return page(s.pages, l), nil
}

func page[T any](rows []T, l content.Listing) []T {
	from := (l.Page - 1) * l.PerPage
	if from >= len(rows) {
		return nil
	}
	to := min(from+l.PerPage, len(rows))
	return rows[from:to]
}

func (s *stub) PostByID(_ context.Context, id uuid.UUID, _ content.Audience) (*content.Post, error) {
	for i := range s.posts {
		if s.posts[i].ID == id {
			p := s.posts[i]
			return &p, nil
		}
	}
	return nil, content.ErrNotFound
}

func (s *stub) PageByID(_ context.Context, id uuid.UUID, _ content.Audience) (*content.Page, error) {
	for i := range s.pages {
		if s.pages[i].ID == id {
			p := s.pages[i]
			return &p, nil
		}
	}
	return nil, content.ErrNotFound
}

func (s *stub) CreatePost(_ context.Context, p *content.Post) error {
	if s.err != nil {
		return s.err
	}
	p.ID = uuid.Must(uuid.NewV7())
	s.posts = append(s.posts, *p)
	s.writes++
	return nil
}

func (s *stub) UpdatePost(_ context.Context, p *content.Post) error {
	if s.err != nil {
		return s.err
	}
	for i := range s.posts {
		if s.posts[i].ID == p.ID {
			s.posts[i] = *p
		}
	}
	s.writes++
	return nil
}

func (s *stub) CreatePage(_ context.Context, p *content.Page) error {
	if s.err != nil {
		return s.err
	}
	p.ID = uuid.Must(uuid.NewV7())
	s.pages = append(s.pages, *p)
	s.writes++
	return nil
}

func (s *stub) UpdatePage(_ context.Context, p *content.Page) error {
	if s.err != nil {
		return s.err
	}
	for i := range s.pages {
		if s.pages[i].ID == p.ID {
			s.pages[i] = *p
		}
	}
	s.writes++
	return nil
}

// testPassword is hashed once for the whole package: argon2id is expensive by
// design, and paying for it per test would make this suite the slow one it
// exists to avoid being.
const testPassword = "correct horse battery staple"

var testHash = mustHash(testPassword)

func mustHash(pw string) string {
	h, err := admin.HashPassword(pw)
	if err != nil {
		panic(err)
	}
	return h
}

func testConfig() *config.Config {
	return &config.Config{Admin: config.AdminConfig{
		PasswordHash:   config.Secret(testHash),
		SessionSecret:  config.Secret("a-signing-key-for-tests"),
		SessionTTL:     7 * 24 * time.Hour,
		InsecureCookie: true,
		Timezone:       "UTC",
		LoginAttempts:  3,
		LoginWindow:    time.Minute,
	}}
}

// newAdmin returns the surface and the store behind it.
func newAdmin(t *testing.T, s *stub) http.Handler {
	t.Helper()
	if s == nil {
		s = &stub{}
	}
	return admin.New(s, testConfig(), slog.New(slog.NewTextHandler(io.Discard, nil)))
}

// logIn returns a handler and the session cookie of an Author who is already in.
func logIn(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()

	body := strings.NewReader("password=" + url(testPassword))
	r := httptest.NewRequest(http.MethodPost, "/admin/login", body)
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)

	for _, c := range w.Result().Cookies() {
		if c.Name == "pw_session" && c.Value != "" {
			return c
		}
	}
	t.Fatalf("logging in did not set a session cookie: status %d", w.Code)
	return nil
}

// do issues a request, optionally carrying a session and the htmx header.
func do(h http.Handler, method, target string, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, target, nil)
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func withSession(c *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(c) }
}

func asHTMX(r *http.Request) { r.Header.Set("HX-Request", "true") }

func form(values string) func(*http.Request) {
	return func(r *http.Request) {
		r.Body = io.NopCloser(strings.NewReader(values))
		r.ContentLength = int64(len(values))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
}

func url(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), "&", "%26")
}

func body(w *httptest.ResponseRecorder) string { return w.Body.String() }
