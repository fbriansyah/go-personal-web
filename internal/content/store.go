package content

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gobuffalo/pop/v6"
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

// Posts returns published Posts newest first, one page at a time. Page numbers
// start at 1.
func (s *Store) Posts(ctx context.Context, aud Audience, page, perPage int) ([]Post, error) {
	posts := []Post{}
	q := s.visible(ctx, aud).Order("published_at desc nulls first, created_at desc")
	if err := q.Paginate(page, perPage).All(&posts); err != nil {
		return nil, wrap(err, "posts")
	}
	return posts, nil
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

// wrap turns pop's "no rows" into ErrNotFound and gives every other failure the
// slug or operation it happened on. Pop errors never escape this package.
func wrap(err error, format string, args ...any) error {
	what := fmt.Sprintf(format, args...)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("%s: %w", what, ErrNotFound)
	}
	return fmt.Errorf("content: %s: %w", what, err)
}
