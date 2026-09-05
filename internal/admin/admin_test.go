package admin_test

import (
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/admin"
	"github.com/fbriansyah/go-personal-web/internal/content"
)

// The single most breakable thing in this design: XHR follows a redirect
// transparently, so answering an htmx request with 303 would have htmx swap the
// login page into whatever hx-target the failed action named — a login form in
// a div, no error anywhere (ADR-0011).
func TestAnExpiredSessionIsAnsweredByShape(t *testing.T) {
	h := newAdmin(t, nil)

	t.Run("an ordinary request is redirected", func(t *testing.T) {
		w := do(h, http.MethodGet, "/admin/posts")

		if w.Code != http.StatusSeeOther {
			t.Fatalf("status = %d, want 303", w.Code)
		}
		if got := w.Header().Get("Location"); !strings.HasPrefix(got, "/admin/login?next=") {
			t.Errorf("Location = %q, want the login form with a return path", got)
		}
	})

	t.Run("an htmx request is not", func(t *testing.T) {
		w := do(h, http.MethodGet, "/admin/posts", asHTMX)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 — a 303 here is swapped into the page", w.Code)
		}
		if got := w.Header().Get("HX-Redirect"); !strings.HasPrefix(got, "/admin/login") {
			t.Errorf("HX-Redirect = %q, want the login form", got)
		}
		if got := w.Header().Get("Location"); got != "" {
			t.Errorf("Location = %q, want none: htmx would follow it", got)
		}
	})
}

// The one branch the whole design rests on: one place decides whether a
// component is framed, and no handler decides for itself (ADR-0011).
func TestOnePlaceDecidesWhetherAScreenIsFramed(t *testing.T) {
	h := newAdmin(t, &stub{posts: []content.Post{draft("first-post")}})
	session := logIn(t, h)

	full := do(h, http.MethodGet, "/admin/posts", withSession(session))
	if !strings.Contains(body(full), "<!doctype html>") && !strings.Contains(body(full), "<!DOCTYPE html>") {
		t.Errorf("an ordinary request should get a whole document:\n%s", truncate(body(full)))
	}

	fragment := do(h, http.MethodGet, "/admin/posts", withSession(session), asHTMX)
	if strings.Contains(fragment.Body.String(), "<html") {
		t.Errorf("an htmx request should get a bare fragment:\n%s", truncate(body(fragment)))
	}
	if !strings.Contains(body(fragment), `id="post-list"`) {
		t.Errorf("the fragment should be the list region:\n%s", truncate(body(fragment)))
	}
	if !strings.Contains(body(fragment), "first-post") {
		t.Errorf("the fragment should carry the rows:\n%s", truncate(body(fragment)))
	}
}

// A rejected save comes back with what was typed, not with what is stored.
// Losing a body to a message about a slug is the one thing this must not do.
func TestARefusedSaveKeepsWhatWasTyped(t *testing.T) {
	store := &stub{err: content.ErrSlugTaken}
	h := newAdmin(t, store)
	session := logIn(t, h)

	w := do(h, http.MethodPost, "/admin/posts",
		withSession(session), asHTMX,
		form("title=A+title&slug=taken&body=Words+worth+keeping"))

	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422", w.Code)
	}
	if !strings.Contains(body(w), "already answers to that slug") {
		t.Errorf("the slug error should be shown:\n%s", truncate(body(w)))
	}
	if !strings.Contains(body(w), "Words worth keeping") {
		t.Errorf("the body should survive the rejection:\n%s", truncate(body(w)))
	}
	if !strings.Contains(body(w), `value="taken"`) {
		t.Errorf("the slug should survive the rejection:\n%s", truncate(body(w)))
	}
	if store.writes != 0 {
		t.Errorf("nothing should have been stored, got %d writes", store.writes)
	}
}

// The limiter stands in front of argon2id, not behind it: behind it, the cost
// an attacker is spending has already been paid (ADR-0015).
func TestLoginAttemptsAreRefusedBeforeTheHashIsComputed(t *testing.T) {
	h := newAdmin(t, nil)

	// testConfig allows three attempts.
	for i := 1; i <= 3; i++ {
		w := do(h, http.MethodPost, "/admin/login", form("password=wrong"))
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: status = %d, want 401", i, w.Code)
		}
	}

	refused := do(h, http.MethodPost, "/admin/login", form("password=wrong"))
	if refused.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", refused.Code)
	}

	// The right password, while refused, must look exactly like the wrong one.
	// Anything else tells an attacker their guess landed.
	correct := do(h, http.MethodPost, "/admin/login", form("password="+url(testPassword)))
	if correct.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 for the right password too", correct.Code)
	}
	for _, c := range correct.Result().Cookies() {
		if c.Name == "pw_session" && c.Value != "" {
			t.Fatal("a refused attempt issued a session")
		}
	}
	if body(refused) != body(correct) {
		t.Error("a refused wrong password and a refused right one read differently")
	}
}

// Deleting a paragraph in order to rewrite it must not publish the gap
// (ADR-0014).
func TestAutosaveRefusesPublishedWriting(t *testing.T) {
	published := published("live-post", -time.Hour)
	store := &stub{posts: []content.Post{published}}
	h := newAdmin(t, store)
	session := logIn(t, h)

	w := do(h, http.MethodPost, "/admin/posts/"+published.ID.String()+"/autosave",
		withSession(session), asHTMX,
		form("title=Half+an+edit&slug=live-post&body=A+paragraph+mid-rewrite"))

	if store.writes != 0 {
		t.Fatalf("published writing was autosaved: %d writes", store.writes)
	}
	if !strings.Contains(body(w), "saved only when you ask") {
		t.Errorf("the marker should say why:\n%s", truncate(body(w)))
	}
	// The marker that comes back carries no trigger, so the browser stops
	// asking rather than looping against a refusal.
	if strings.Contains(body(w), "hx-trigger") {
		t.Errorf("the refusal should stop the loop:\n%s", truncate(body(w)))
	}
}

func TestAutosaveStoresADraft(t *testing.T) {
	d := draft("in-progress")
	store := &stub{posts: []content.Post{d}}
	h := newAdmin(t, store)
	session := logIn(t, h)

	w := do(h, http.MethodPost, "/admin/posts/"+d.ID.String()+"/autosave",
		withSession(session), asHTMX,
		form("title=Still+writing&slug=in-progress&body=More+words"))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if store.writes != 1 {
		t.Fatalf("writes = %d, want 1", store.writes)
	}
	if store.posts[0].Body != "More words" {
		t.Errorf("body = %q", store.posts[0].Body)
	}
	if !strings.Contains(body(w), "hx-trigger") {
		t.Errorf("autosave should keep asking:\n%s", truncate(body(w)))
	}
}

// Publishing is an act the Author performs. A date typed into the form but not
// yet saved must not go live because fifteen seconds passed.
func TestAutosaveNeverPublishes(t *testing.T) {
	d := draft("not-yet")
	store := &stub{posts: []content.Post{d}}
	h := newAdmin(t, store)
	session := logIn(t, h)

	do(h, http.MethodPost, "/admin/posts/"+d.ID.String()+"/autosave",
		withSession(session), asHTMX,
		form("title=Not+yet&slug=not-yet&body=b&published_at=2020-01-01T00%3A00"))

	if store.posts[0].PublishedAt != nil {
		t.Errorf("autosave published the draft: %v", store.posts[0].PublishedAt)
	}
}

// "Publish now" fills in the Publication Date rather than setting a flag: the
// date is the only state there is (ADR-0008), and the button needs no
// JavaScript to do it.
func TestPublishNowSetsTheDate(t *testing.T) {
	d := draft("ready")
	store := &stub{posts: []content.Post{d}}
	h := newAdmin(t, store)
	session := logIn(t, h)

	do(h, http.MethodPost, "/admin/posts/"+d.ID.String(),
		withSession(session),
		form("title=Ready&slug=ready&body=b&published_at=&action=publish_now"))

	if store.posts[0].PublishedAt == nil {
		t.Fatal("publish now left the post a draft")
	}
	if !store.posts[0].IsPublished(time.Now()) {
		t.Errorf("published_at = %v, want a time already passed", store.posts[0].PublishedAt)
	}
}

// Clearing the date is how writing is withdrawn. There is no unpublish button
// because there is no status to unset.
func TestClearingTheDateWithdrawsWriting(t *testing.T) {
	p := published("live", -time.Hour)
	store := &stub{posts: []content.Post{p}}
	h := newAdmin(t, store)
	session := logIn(t, h)

	do(h, http.MethodPost, "/admin/posts/"+p.ID.String(),
		withSession(session),
		form("title=Live&slug=live&body=b&published_at=&action=save"))

	if store.posts[0].PublishedAt != nil {
		t.Errorf("clearing the date left it published: %v", store.posts[0].PublishedAt)
	}
}

// The return path comes from the URL, so it has to be treated as something a
// stranger wrote.
func TestTheReturnPathCannotLeaveAdmin(t *testing.T) {
	h := newAdmin(t, nil)

	for _, next := range []string{"https://elsewhere.example/", "//elsewhere.example/", "/etc/passwd"} {
		t.Run(next, func(t *testing.T) {
			w := do(h, http.MethodPost, "/admin/login",
				form("password="+url(testPassword)+"&next="+next))

			if got := w.Header().Get("Location"); !strings.HasPrefix(got, "/admin") {
				t.Errorf("Location = %q, want somewhere inside /admin", got)
			}
		})
	}
}

// Logging out is all it can be: the cookie leaves this browser. There is no
// session storage to revoke against (ADR-0009).
func TestLogoutClearsTheCookie(t *testing.T) {
	h := newAdmin(t, nil)
	session := logIn(t, h)

	w := do(h, http.MethodPost, "/admin/logout", withSession(session))
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want 303", w.Code)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == "pw_session" {
			if c.MaxAge >= 0 || c.Value != "" {
				t.Errorf("cookie = %+v, want it removed", c)
			}
			return
		}
	}
	t.Error("logout set no cookie")
}

// A forged or truncated cookie is not a session, and says nothing about why.
func TestATamperedCookieIsNoSession(t *testing.T) {
	h := newAdmin(t, nil)
	session := logIn(t, h)

	for name, value := range map[string]string{
		"a flipped signature": session.Value[:len(session.Value)-1] + "x",
		"no signature":        strings.Split(session.Value, ".")[0],
		"nonsense":            "not-a-cookie",
		"empty":               "",
	} {
		t.Run(name, func(t *testing.T) {
			forged := &http.Cookie{Name: "pw_session", Value: value}
			w := do(h, http.MethodGet, "/admin/posts", withSession(forged))
			if w.Code != http.StatusSeeOther {
				t.Errorf("status = %d, want 303 to the login form", w.Code)
			}
		})
	}
}

// The cookie is the whole of the CSRF defence, so its attributes are the
// defence and belong in a test (ADR-0015).
func TestTheSessionCookieIsStrictAndHttpOnly(t *testing.T) {
	h := newAdmin(t, nil)
	c := logIn(t, h)

	if c.SameSite != http.SameSiteStrictMode {
		t.Errorf("SameSite = %v, want Strict — it is the CSRF defence", c.SameSite)
	}
	if !c.HttpOnly {
		t.Error("the cookie should be HttpOnly")
	}
	if c.Path != "/admin" {
		t.Errorf("Path = %q, want /admin", c.Path)
	}
}

// Assets are ours, which is what makes this policy affordable (ADR-0013).
func TestEveryAdminResponseCarriesTheContentSecurityPolicy(t *testing.T) {
	h := newAdmin(t, nil)

	w := do(h, http.MethodGet, "/admin/login")
	csp := w.Header().Get("Content-Security-Policy")
	for _, want := range []string{"default-src 'self'", "script-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("policy = %q, want it to contain %q", csp, want)
		}
	}
	if strings.Contains(csp, "unsafe-inline") || strings.Contains(csp, "unsafe-eval") {
		t.Errorf("policy = %q, want neither unsafe-inline nor unsafe-eval", csp)
	}
}

func TestTheEmbeddedAssetsAreServed(t *testing.T) {
	h := newAdmin(t, nil)

	for path, wantType := range map[string]string{
		"/admin/static/htmx.min.js": "text/javascript",
		"/admin/static/app.css":     "text/css",
	} {
		w := do(h, http.MethodGet, path)
		if w.Code != http.StatusOK {
			t.Errorf("%s: status = %d", path, w.Code)
			continue
		}
		if got := w.Header().Get("Content-Type"); !strings.HasPrefix(got, wantType) {
			t.Errorf("%s: Content-Type = %q, want %q", path, got, wantType)
		}
		if w.Body.Len() == 0 {
			t.Errorf("%s: empty", path)
		}
	}

	// Only the two files are reachable. Serving the directory would make every
	// file added to it public by default.
	if w := do(h, http.MethodGet, "/admin/static/../admin.go"); w.Code == http.StatusOK {
		t.Error("the asset handler served something it should not")
	}
}

// A search has to survive a reload, which means the filter lives in the URL and
// the fragment carries it forward (ADR-0011).
func TestSearchKeepsItsQueryInTheLoadMoreLink(t *testing.T) {
	var posts []content.Post
	for i := range 60 {
		posts = append(posts, draft("post-"+string(rune('a'+i%26))+string(rune('a'+i/26))))
	}
	h := newAdmin(t, &stub{posts: posts})
	session := logIn(t, h)

	w := do(h, http.MethodGet, "/admin/posts?q=post", withSession(session), asHTMX)
	if !strings.Contains(body(w), "q=post&amp;page=2") && !strings.Contains(body(w), "q=post&page=2") {
		t.Errorf("load more should carry the query:\n%s", truncate(body(w)))
	}
}

// A later page is rows to append, not a whole region: appending a region would
// nest the list inside itself.
func TestALaterPageIsRowsAlone(t *testing.T) {
	var posts []content.Post
	for i := range 60 {
		posts = append(posts, draft("post-"+string(rune('a'+i%26))+string(rune('a'+i/26))))
	}
	h := newAdmin(t, &stub{posts: posts})
	session := logIn(t, h)

	w := do(h, http.MethodGet, "/admin/posts?page=2", withSession(session), asHTMX)
	if strings.Contains(body(w), `id="post-list"`) {
		t.Errorf("page 2 should be rows, not the region:\n%s", truncate(body(w)))
	}
}

func TestAnUnknownIDIsNotFound(t *testing.T) {
	h := newAdmin(t, nil)
	session := logIn(t, h)

	for _, path := range []string{
		"/admin/posts/" + uuid.Must(uuid.NewV7()).String() + "/edit",
		"/admin/posts/not-a-uuid/edit",
	} {
		if w := do(h, http.MethodGet, path, withSession(session)); w.Code != http.StatusNotFound {
			t.Errorf("%s: status = %d, want 404", path, w.Code)
		}
	}
}

func draft(slug string) content.Post {
	return content.Post{
		ID:    uuid.Must(uuid.NewV7()),
		Slug:  slug,
		Title: "Title of " + slug,
		Body:  "Body",
	}
}

func published(slug string, ago time.Duration) content.Post {
	p := draft(slug)
	at := time.Now().Add(ago)
	p.PublishedAt = &at
	return p
}

func truncate(s string) string {
	if len(s) > 1200 {
		return s[:1200] + "…"
	}
	return s
}

// The expiry is enforced by the server from the signed payload, not by the
// browser from Max-Age. Max-Age is an instruction anyone holding a copy of the
// cookie can ignore; the signed copy is the one that gives a stolen cookie a
// life (ADR-0015).
func TestExpiryIsEnforcedByTheServer(t *testing.T) {
	cfg := testConfig()
	cfg.Admin.SessionTTL = time.Nanosecond
	h := admin.New(&stub{}, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))

	w := do(h, http.MethodPost, "/admin/login", form("password="+url(testPassword)))
	var session *http.Cookie
	for _, c := range w.Result().Cookies() {
		if c.Name == "pw_session" && c.Value != "" {
			session = c
		}
	}
	if session == nil {
		t.Fatal("no session was issued")
	}

	// The browser still holds the cookie and still sends it. The server is what
	// refuses it.
	if got := do(h, http.MethodGet, "/admin/posts", withSession(session)); got.Code != http.StatusSeeOther {
		t.Errorf("status = %d, want 303: an expired session must not be honoured", got.Code)
	}
}
