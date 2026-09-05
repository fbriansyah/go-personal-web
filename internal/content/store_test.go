package content_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/content"
)

// newPost builds a valid Post with the given slug and Publication Date.
func newPost(slug string, publishedAt *time.Time) *content.Post {
	return &content.Post{
		Slug:        slug,
		Title:       "Title of " + slug,
		Body:        "# " + slug + "\n\nBody in Markdown.",
		PublishedAt: publishedAt,
	}
}

// The Publication Date carries every state, so the public filter has to read it
// three different ways from one column.
func TestPostBySlugAppliesThePublicationDate(t *testing.T) {
	tests := []struct {
		name        string
		publishedAt *time.Time
		publicSees  bool
	}{
		{"a draft is hidden", nil, false},
		{"scheduled for later is hidden", at(time.Hour), false},
		{"published in the past is visible", at(-time.Hour), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := newStore(t)

			post := newPost("state-of-the-post", tt.publishedAt)
			if err := store.CreatePost(ctx, post); err != nil {
				t.Fatalf("CreatePost: %v", err)
			}

			_, err := store.PostBySlug(ctx, post.Slug, content.Public)
			switch {
			case tt.publicSees && err != nil:
				t.Errorf("public should see this post: %v", err)
			case !tt.publicSees && !errors.Is(err, content.ErrNotFound):
				t.Errorf("public should get ErrNotFound, got %v", err)
			}

			// The Author previews through the same query, so whatever the
			// state, it is reachable.
			got, err := store.PostBySlug(ctx, post.Slug, content.Author)
			if err != nil {
				t.Fatalf("author should always see the post: %v", err)
			}
			if got.Body != post.Body {
				t.Errorf("body = %q, want %q", got.Body, post.Body)
			}
		})
	}
}

// PostgreSQL evaluates now() at the start of the transaction, not at the
// statement. That is the behaviour we want — every read in one request sees the
// same instant — but it means a scheduled Post cannot "arrive" mid-transaction,
// and no amount of sleeping inside one will make it visible.
func TestPublicationBoundaryIsTheTransactionsClock(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	if err := store.CreatePost(ctx, newPost("just-missed", at(50*time.Millisecond))); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if err := store.CreatePost(ctx, newPost("just-made-it", at(-50*time.Millisecond))); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if _, err := store.PostBySlug(ctx, "just-missed", content.Public); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("now() should still be the transaction's start time, got %v", err)
	}
	if _, err := store.PostBySlug(ctx, "just-made-it", content.Public); err != nil {
		t.Errorf("a post published before the transaction began should be visible: %v", err)
	}
}

func TestPostsListsPublishedNewestFirst(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for slug, published := range map[string]*time.Time{
		"oldest": at(-72 * time.Hour),
		"middle": at(-48 * time.Hour),
		"newest": at(-24 * time.Hour),
		"draft":  nil,
		"future": at(24 * time.Hour),
	} {
		if err := store.CreatePost(ctx, newPost(slug, published)); err != nil {
			t.Fatalf("CreatePost(%s): %v", slug, err)
		}
	}

	posts, err := store.Posts(ctx, content.Public, 1, 10)
	if err != nil {
		t.Fatalf("Posts: %v", err)
	}

	want := []string{"newest", "middle", "oldest"}
	if len(posts) != len(want) {
		t.Fatalf("got %d posts, want %d: %v", len(posts), len(want), slugs(posts))
	}
	for i, slug := range want {
		if posts[i].Slug != slug {
			t.Errorf("posts[%d] = %q, want %q (got %v)", i, posts[i].Slug, slug, slugs(posts))
		}
	}
}

func TestPostsPaginates(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for i, slug := range []string{"first", "second", "third"} {
		published := at(-time.Duration(3-i) * time.Hour)
		if err := store.CreatePost(ctx, newPost(slug, published)); err != nil {
			t.Fatalf("CreatePost(%s): %v", slug, err)
		}
	}

	page1, err := store.Posts(ctx, content.Public, 1, 2)
	if err != nil {
		t.Fatalf("Posts page 1: %v", err)
	}
	page2, err := store.Posts(ctx, content.Public, 2, 2)
	if err != nil {
		t.Fatalf("Posts page 2: %v", err)
	}

	if got := slugs(page1); len(got) != 2 || got[0] != "third" || got[1] != "second" {
		t.Errorf("page 1 = %v, want [third second]", got)
	}
	if got := slugs(page2); len(got) != 1 || got[0] != "first" {
		t.Errorf("page 2 = %v, want [first]", got)
	}
}

// Pop fills an empty UUID with v4 and cannot be asked for anything else, so
// this is the guard on the BeforeCreate hook that gets us v7 (ADR-0006).
// Without it, losing the hook is silent.
func TestCreateAssignsTimeOrderedUUIDv7(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	var ids []uuid.UUID
	for _, slug := range []string{"one", "two", "three"} {
		post := newPost(slug, at(-time.Hour))
		if err := store.CreatePost(ctx, post); err != nil {
			t.Fatalf("CreatePost(%s): %v", slug, err)
		}
		if got := post.ID.Version(); got != 7 {
			t.Fatalf("id %s is UUIDv%d, want v7 — the BeforeCreate hook is gone", post.ID, got)
		}
		ids = append(ids, post.ID)
	}

	// The reason for v7 over v4 is that identifiers sort by creation.
	for i := 1; i < len(ids); i++ {
		if ids[i].String() <= ids[i-1].String() {
			t.Errorf("ids are not time-ordered: %s came after %s", ids[i], ids[i-1])
		}
	}
}

// An ID set deliberately is left alone, which is how an import or a fixture
// pins one.
func TestCreateKeepsAnExplicitID(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	id := uuid.Must(uuid.NewV7())
	post := newPost("pinned", at(-time.Hour))
	post.ID = id

	if err := store.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if post.ID != id {
		t.Errorf("id = %s, want %s", post.ID, id)
	}
}

func TestPagesAreAddressedBySlug(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	page := &content.Page{
		Slug:        "about",
		Title:       "About",
		Body:        "Who I am.",
		PublishedAt: at(-time.Hour),
	}
	if err := store.CreatePage(ctx, page); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	got, err := store.PageBySlug(ctx, "about", content.Public)
	if err != nil {
		t.Fatalf("PageBySlug: %v", err)
	}
	if got.Title != "About" || got.ID.Version() != 7 {
		t.Errorf("got %+v", got)
	}

	if _, err := store.PageBySlug(ctx, "missing", content.Public); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("missing page should be ErrNotFound, got %v", err)
	}
}

// A Slug is the only identifier a visitor sees, so the schema refuses a shape
// that could not be a URL.
//
// Each case gets its own transaction: a constraint violation aborts the one it
// happens in, and every statement after it fails with the abort rather than
// with what the test meant to check.
func TestSchemaRejectsAMalformedSlug(t *testing.T) {
	for _, slug := range []string{"Has Capitals", "trailing-", "under_score", "", "with spaces"} {
		t.Run(slug, func(t *testing.T) {
			if err := newStore(t).CreatePost(context.Background(), newPost(slug, nil)); err == nil {
				t.Errorf("slug %q was accepted", slug)
			}
		})
	}
}

func TestSchemaRejectsADuplicateSlug(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	if err := store.CreatePost(ctx, newPost("taken", nil)); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	if err := store.CreatePost(ctx, newPost("taken", nil)); err == nil {
		t.Error("duplicate slug was accepted")
	}
}

// Pop errors carry the driver's vocabulary; callers outside this package see
// only content's own.
func TestNotFoundIsTheOnlyErrorForAMissingPost(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	_, err := store.PostBySlug(ctx, "never-written", content.Author)
	if !errors.Is(err, content.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdatePostRefreshesUpdatedAt(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	post := newPost("editable", at(-time.Hour))
	if err := store.CreatePost(ctx, post); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}
	before := post.UpdatedAt

	post.Title = "A better title"
	if err := store.UpdatePost(ctx, post); err != nil {
		t.Fatalf("UpdatePost: %v", err)
	}
	if !post.UpdatedAt.After(before) {
		t.Errorf("updated_at did not move: %s", post.UpdatedAt)
	}

	got, err := store.PostBySlug(ctx, post.Slug, content.Public)
	if err != nil {
		t.Fatalf("PostBySlug: %v", err)
	}
	if got.Title != "A better title" {
		t.Errorf("title = %q, want %q", got.Title, "A better title")
	}
}

func slugs(posts []content.Post) []string {
	out := make([]string, len(posts))
	for i, p := range posts {
		out[i] = p.Slug
	}
	return out
}
