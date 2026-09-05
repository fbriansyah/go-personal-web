// Package content owns the writing on the site: Posts, Pages, and the store
// they are read from and written to.
//
// Pop reaches this package and no further (ADR-0004). Nothing outside it sees a
// *pop.Connection, a pop model tag, or a pop error; callers get domain types
// and the errors declared here.
package content

import (
	"fmt"
	"time"

	"github.com/gobuffalo/pop/v6"
	"github.com/gofrs/uuid"
)

// Audience decides whether unpublished writing is visible.
//
// It is a parameter rather than a second set of methods so that a Draft is
// previewed through the very query that will serve it once published: what the
// Author sees is what visitors will see, because it is the same code path.
type Audience int

const (
	// Public sees only writing whose Publication Date has arrived.
	Public Audience = iota
	// Author sees everything, including Drafts and scheduled writing.
	Author
)

// Post is a dated piece of writing that appears in the chronological index.
type Post struct {
	ID    uuid.UUID `db:"id"`
	Slug  string    `db:"slug"`
	Title string    `db:"title"`
	// Body is Markdown, and the only stored form of it. Rendering happens on
	// read, so the renderer can be changed without a backfill (ADR-0008).
	Body string `db:"body"`
	// PublishedAt nil is a Draft, in the future is scheduled, in the past is
	// live. This one field carries every state; there is no status column.
	PublishedAt *time.Time `db:"published_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

// TableName is stated rather than inferred so that renaming the Go type cannot
// silently rename the table.
func (Post) TableName() string { return "posts" }

// BeforeCreate assigns a time-ordered UUIDv7.
//
// This is not redundant with pop. Pop fills an empty UUID with NewV4 and offers
// no way to ask for another version, and it only does so when the ID is still
// zero — so setting it first is the only way to get v7 (ADR-0006). Deleting
// this method does not break anything visibly; it silently reverts to v4.
func (p *Post) BeforeCreate(*pop.Connection) error { return assignV7(&p.ID) }

// IsPublished reports whether the Post is visible to the public as of now.
func (p *Post) IsPublished(now time.Time) bool {
	return p.PublishedAt != nil && !p.PublishedAt.After(now)
}

// IsDraft reports whether the Post has no Publication Date at all.
func (p *Post) IsDraft() bool { return p.PublishedAt == nil }

// Page is a standalone piece of writing addressed only by its Slug and never
// listed chronologically.
type Page struct {
	ID          uuid.UUID  `db:"id"`
	Slug        string     `db:"slug"`
	Title       string     `db:"title"`
	Body        string     `db:"body"`
	PublishedAt *time.Time `db:"published_at"`
	CreatedAt   time.Time  `db:"created_at"`
	UpdatedAt   time.Time  `db:"updated_at"`
}

func (Page) TableName() string { return "pages" }

// BeforeCreate assigns a time-ordered UUIDv7. See Post.BeforeCreate.
func (p *Page) BeforeCreate(*pop.Connection) error { return assignV7(&p.ID) }

// IsPublished reports whether the Page is visible to the public as of now.
func (p *Page) IsPublished(now time.Time) bool {
	return p.PublishedAt != nil && !p.PublishedAt.After(now)
}

// IsDraft reports whether the Page has no Publication Date at all.
func (p *Page) IsDraft() bool { return p.PublishedAt == nil }

// assignV7 fills id with a UUIDv7 unless one was set deliberately, which is how
// a caller pins an identifier for a fixture or an import.
func assignV7(id *uuid.UUID) error {
	if *id != uuid.Nil {
		return nil
	}
	v, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("content: generating id: %w", err)
	}
	*id = v
	return nil
}
