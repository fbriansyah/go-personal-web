package content

import (
	"context"
	"strings"

	"github.com/gobuffalo/pop/v6"
	"github.com/gofrs/uuid"
)

// publishedClause is the filter every public read applies. now() is evaluated
// per query, which is why a scheduled Post appears without anything running.
const publishedClause = "published_at is not null and published_at <= now()"

// Store reads and writes the site's content.
//
// It is a concrete type, not an interface: there is one implementation, and a
// fake standing in for PostgreSQL would test the half that does not break. The
// consequence is that its tests need a real database (ADR-0004).
type Store struct {
	conn *pop.Connection
}

// NewStore wraps a connection. The connection may be a transaction — pop's
// NewTransaction returns a *pop.Connection — which is how tests isolate cases.
func NewStore(conn *pop.Connection) *Store { return &Store{conn: conn} }

// PostBySlug returns the Post addressed by slug, or ErrNotFound if there is
// none the Audience may see.
func (s *Store) PostBySlug(ctx context.Context, slug string, aud Audience) (*Post, error) {
	var post Post
	q := s.visible(ctx, aud).Where("slug = ?", slug)
	if err := q.First(&post); err != nil {
		return nil, wrap(err, "post %q", slug)
	}
	return &post, nil
}

// Posts returns Posts newest first, one page at a time. Page numbers start
// at 1. It is FindPosts without a filter, and the form the site's own index
// reads through.
func (s *Store) Posts(ctx context.Context, aud Audience, page, perPage int) ([]Post, error) {
	return s.FindPosts(ctx, aud, Listing{Page: page, PerPage: perPage})
}

// Listing narrows a list of writing. Its zero value lists the first page of
// everything, so a caller that wants no filter states nothing.
type Listing struct {
	// Text matches the title or the Slug as a substring, case-insensitively.
	// Empty matches everything.
	//
	// Substring rather than full text search: this exists so the Author can
	// reach a piece of writing they already know exists, and typing "kubern"
	// should find it. Full text search matches whole words only, which is the
	// wrong behaviour for a box that filters as you type.
	Text string
	// Page numbers start at 1. Zero means the first.
	Page int
	// PerPage zero means DefaultPerPage.
	PerPage int
}

// DefaultPerPage is the page size a Listing takes when it names none.
const DefaultPerPage = 50

func (l Listing) page() int {
	if l.Page < 1 {
		return 1
	}
	return l.Page
}

func (l Listing) perPage() int {
	if l.PerPage < 1 {
		return DefaultPerPage
	}
	return l.PerPage
}

// FindPosts returns Posts newest first, filtered by the Listing.
//
// It takes an Audience like every other read: the Admin list is the one caller
// that passes Author, and making that a parameter rather than a separate method
// is what stops a public listing from ever reaching this by accident.
func (s *Store) FindPosts(ctx context.Context, aud Audience, l Listing) ([]Post, error) {
	posts := []Post{}
	q := s.visible(ctx, aud)
	q = matching(q, l.Text)
	// Drafts sort first: in the Admin list the unfinished work is what you came
	// for, and in a public list there are none.
	q = q.Order("published_at desc nulls first, created_at desc")
	if err := q.Paginate(l.page(), l.perPage()).All(&posts); err != nil {
		return nil, wrap(err, "posts")
	}
	return posts, nil
}

// FindPages returns Pages, filtered by the Listing.
//
// Pages have no chronology, so they are ordered by Slug — the name the Author
// looks for them under.
func (s *Store) FindPages(ctx context.Context, aud Audience, l Listing) ([]Page, error) {
	pages := []Page{}
	q := matching(s.visible(ctx, aud), l.Text).Order("slug asc")
	if err := q.Paginate(l.page(), l.perPage()).All(&pages); err != nil {
		return nil, wrap(err, "pages")
	}
	return pages, nil
}

// matching narrows q to rows whose title or Slug contains text. An empty text
// adds no clause at all rather than a clause that matches everything.
func matching(q *pop.Query, text string) *pop.Query {
	text = strings.TrimSpace(text)
	if text == "" {
		return q
	}
	// The pattern is a bound parameter, so a % or _ typed by the Author is
	// matched literally by the escape below rather than acting as a wildcard.
	pattern := "%" + escapeLike(text) + "%"
	return q.Where("(title ilike ? escape '\\' or slug ilike ? escape '\\')", pattern, pattern)
}

// escapeLike neutralises the wildcards of a LIKE pattern.
func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`)
	return r.Replace(s)
}

// PostByID returns the Post with this ID, or ErrNotFound if there is none the
// Audience may see.
//
// Admin addresses writing by ID rather than by Slug (ADR-0012): the Slug is the
// one field the edit form changes, so it cannot also be the address of the form.
func (s *Store) PostByID(ctx context.Context, id uuid.UUID, aud Audience) (*Post, error) {
	var post Post
	if err := s.visible(ctx, aud).Where("id = ?", id).First(&post); err != nil {
		return nil, wrap(err, "post %s", id)
	}
	return &post, nil
}

// PageByID returns the Page with this ID, or ErrNotFound if there is none the
// Audience may see.
func (s *Store) PageByID(ctx context.Context, id uuid.UUID, aud Audience) (*Page, error) {
	var pg Page
	if err := s.visible(ctx, aud).Where("id = ?", id).First(&pg); err != nil {
		return nil, wrap(err, "page %s", id)
	}
	return &pg, nil
}

// PageBySlug returns the Page addressed by slug, or ErrNotFound if there is
// none the Audience may see.
func (s *Store) PageBySlug(ctx context.Context, slug string, aud Audience) (*Page, error) {
	var pg Page
	q := s.visible(ctx, aud).Where("slug = ?", slug)
	if err := q.First(&pg); err != nil {
		return nil, wrap(err, "page %q", slug)
	}
	return &pg, nil
}

// CreatePost stores a new Post, assigning its ID and timestamps.
func (s *Store) CreatePost(ctx context.Context, post *Post) error {
	if err := s.conn.WithContext(ctx).Create(post); err != nil {
		return wrap(err, "creating post %q", post.Slug)
	}
	return nil
}

// UpdatePost stores changes to an existing Post and refreshes its UpdatedAt.
func (s *Store) UpdatePost(ctx context.Context, post *Post) error {
	if err := s.conn.WithContext(ctx).Update(post); err != nil {
		return wrap(err, "updating post %q", post.Slug)
	}
	return nil
}

// CreatePage stores a new Page, assigning its ID and timestamps.
func (s *Store) CreatePage(ctx context.Context, pg *Page) error {
	if err := s.conn.WithContext(ctx).Create(pg); err != nil {
		return wrap(err, "creating page %q", pg.Slug)
	}
	return nil
}

// UpdatePage stores changes to an existing Page and refreshes its UpdatedAt.
func (s *Store) UpdatePage(ctx context.Context, pg *Page) error {
	if err := s.conn.WithContext(ctx).Update(pg); err != nil {
		return wrap(err, "updating page %q", pg.Slug)
	}
	return nil
}

// visible starts a query already narrowed to what aud is allowed to see, so
// that no read can forget the filter by omission.
func (s *Store) visible(ctx context.Context, aud Audience) *pop.Query {
	c := s.conn.WithContext(ctx)
	if aud == Author {
		return c.Q()
	}
	return c.Where(publishedClause)
}
