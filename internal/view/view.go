// Package view holds every templ component the site renders.
//
// It cannot reach the store (ADR-0010): nothing here imports content.Store,
// takes a *http.Request, or performs a query. Components receive ordinary Go
// values, assembled by a handler that has already fetched everything a screen
// needs. templ makes querying from inside a template both possible and
// pleasant, and this package boundary is what makes it impossible instead.
//
// templ's `css` expressions are not used, in this package or anywhere: they
// emit a <style> block, which the Content-Security-Policy in ADR-0013 forbids.
// Styling lives in the embedded stylesheet.
package view

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/fbriansyah/go-personal-web/internal/content"
)

// htmxConfig is read by htmx from a <meta> tag, which is not a script and so
// needs no exception in the Content-Security-Policy.
//
// The 422 entry is the reason this exists. htmx does not swap responses with an
// error status by default, and a failed validation answers 422 with the form
// and its messages (ADR-0011). Answering 200 for a failure was rejected: it
// would lie to the logs and to the tests.
const htmxConfig = `{"allowEval":false,"responseHandling":[` +
	`{"code":"422","swap":true},` +
	`{"code":"204","swap":false},` +
	`{"code":"[23]..","swap":true},` +
	`{"code":"[45]..","swap":false,"error":true}]}`

// Nav names the Admin section a screen belongs to, so the layout can mark it.
type Nav string

const (
	NavPosts Nav = "posts"
	NavPages Nav = "pages"
	NavNone  Nav = ""
)

// Listing is everything a list screen shows.
type Listing struct {
	// Query is the text the search box currently holds.
	Query string
	// NextPage is the page the "load more" button asks for, or 0 when the
	// listing is complete. It is decided by the handler fetching one row more
	// than it shows, which is how the button appears without a count query.
	NextPage int
	// Zone is the timezone every date on the screen is rendered in. It is
	// stated in config rather than taken from the server, so moving a
	// deployment cannot shift what a schedule appears to say.
	Zone *time.Location
}

// PostListing is a list of Posts and the state of the screen showing them.
type PostListing struct {
	Listing
	Posts []content.Post
}

// PageListing is a list of Pages and the state of the screen showing them.
type PageListing struct {
	Listing
	Pages []content.Page
}

// Form is the state of an edit screen: the values as the Author last left them,
// not as the database holds them, so a rejected save comes back with what was
// typed rather than with what was stored.
type Form struct {
	// ID is empty for writing that has no row yet. A new Post becomes a row on
	// the first explicit save (ADR-0014), so this is empty exactly once.
	ID    string
	Slug  string
	Title string
	Body  string
	// PublishedAt is the value of the datetime-local input: either empty, or
	// "2006-01-02T15:04" in Zone.
	PublishedAt string
	// Errors are keyed by field name — "slug", "title", or "" for the form as
	// a whole.
	Errors map[string]string
	// Zone labels the datetime input, so the Author never has to guess which
	// zone the time they typed is understood in.
	Zone string
	// Autosave is on only for Drafts. Published writing changes when the Author
	// asks and not before, because someone may be reading it (ADR-0014).
	Autosave bool
	// Kind is "post" or "page"; it decides where the form posts to and what it
	// is called on screen.
	Kind string
}

// Err returns the message for a field, or the empty string.
func (f Form) Err(field string) string { return f.Errors[field] }

// IsNew reports whether this writing has no row yet.
func (f Form) IsNew() bool { return f.ID == "" }

// LoginForm is the state of the login screen.
type LoginForm struct {
	// Next is the screen to return to once the Author is back in.
	Next string
	// Error is shown when a login was refused. It never distinguishes a wrong
	// password from a rate-limited attempt (ADR-0015).
	Error string
}

// state describes a piece of writing in one word, for the list screens. There
// is no status column — the Publication Date carries every state (ADR-0008) —
// so this is derived, never stored.
func state(publishedAt *time.Time, now time.Time) string {
	switch {
	case publishedAt == nil:
		return "draft"
	case publishedAt.After(now):
		return "scheduled"
	default:
		return "published"
	}
}

// when renders a Publication Date in the Author's zone, or says there is none.
func when(publishedAt *time.Time, zone *time.Location) string {
	if publishedAt == nil {
		return "—"
	}
	if zone == nil {
		zone = time.UTC
	}
	return publishedAt.In(zone).Format("2 Jan 2006, 15:04")
}

// PostState and PostWhen exist because a templ expression calls a function
// rather than writing a switch.
func PostState(p content.Post, now time.Time) string   { return state(p.PublishedAt, now) }
func PageState(p content.Page, now time.Time) string   { return state(p.PublishedAt, now) }
func PostWhen(p content.Post, z *time.Location) string { return when(p.PublishedAt, z) }
func PageWhen(p content.Page, z *time.Location) string { return when(p.PublishedAt, z) }

// listURL builds the URL a list screen's search and paging use. One URL serves
// the whole page, the table after a search, and the next page of rows; which
// one comes back is decided by the request, not by a second endpoint.
func listURL(base, query string, page int) string {
	var b strings.Builder
	b.WriteString(base)
	sep := "?"
	if query != "" {
		b.WriteString(sep + "q=" + url.QueryEscape(query))
		sep = "&"
	}
	if page > 1 {
		b.WriteString(sep + "page=" + strconv.Itoa(page))
	}
	return b.String()
}

// indexPath is the list a piece of writing belongs to.
func indexPath(kind string) string { return "/admin/" + kind + "s" }

// savePath is where the editor posts.
//
// New writing posts to the collection and existing writing to its own ID: Admin
// addresses writing by ID and never by Slug, because the Slug is the one field
// this form changes (ADR-0012).
func savePath(f Form) string {
	if f.IsNew() {
		return "/admin/" + f.Kind + "s"
	}
	return "/admin/" + f.Kind + "s/" + f.ID
}

// autosavePath is the endpoint the Draft saves itself to.
func autosavePath(f Form) string {
	return "/admin/" + f.Kind + "s/" + f.ID + "/autosave"
}
