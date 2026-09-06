package content_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gofrs/uuid"

	"github.com/fbriansyah/go-personal-web/internal/content"
)

// The schema enforces the rules; this package's job is to say which one broke
// in the domain's words. Without this, the Author's most common mistake — a
// slug already in use — reaches the form as a driver error naming a constraint.
func TestConstraintViolationsBecomeDomainErrors(t *testing.T) {
	tests := []struct {
		name  string
		post  *content.Post
		first *content.Post
		want  error
	}{
		{
			name:  "a slug already in use",
			first: newPost("taken", nil),
			post:  newPost("taken", nil),
			want:  content.ErrSlugTaken,
		},
		{
			name: "a slug that could not be a URL",
			post: &content.Post{Slug: "Not A Slug", Title: "Title", Body: "Body"},
			want: content.ErrSlugInvalid,
		},
		{
			name: "a title of nothing but spaces",
			post: &content.Post{Slug: "blank-title", Title: "   ", Body: "Body"},
			want: content.ErrTitleRequired,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// A constraint violation aborts its transaction, so each case needs
			// its own.
			ctx := context.Background()
			store := newStore(t)

			if tt.first != nil {
				if err := store.CreatePost(ctx, tt.first); err != nil {
					t.Fatalf("CreatePost(first): %v", err)
				}
			}
			err := store.CreatePost(ctx, tt.post)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

// Pages carry the same constraints under different names, so the mapping has to
// match by suffix rather than by table.
func TestPageConstraintViolationsBecomeDomainErrors(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	if err := store.CreatePage(ctx, &content.Page{Slug: "about", Title: "About", Body: "b"}); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}
	err := store.CreatePage(ctx, &content.Page{Slug: "about", Title: "About again", Body: "b"})
	if !errors.Is(err, content.ErrSlugTaken) {
		t.Fatalf("err = %v, want ErrSlugTaken", err)
	}
}

// Admin reaches writing by ID (ADR-0012), and that lookup obeys the Audience
// like every other read — otherwise it would be a way to serve a Draft.
func TestByIDObeysTheAudience(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	draft := newPost("a-draft", nil)
	if err := store.CreatePost(ctx, draft); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	if _, err := store.PostByID(ctx, draft.ID, content.Public); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("public should not reach a draft by id, got %v", err)
	}
	got, err := store.PostByID(ctx, draft.ID, content.Author)
	if err != nil {
		t.Fatalf("PostByID as author: %v", err)
	}
	if got.Slug != "a-draft" {
		t.Errorf("slug = %q", got.Slug)
	}

	if _, err := store.PostByID(ctx, uuid.Must(uuid.NewV7()), content.Author); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("an unknown id should be ErrNotFound, got %v", err)
	}
}

func TestPageByIDObeysTheAudience(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	pg := &content.Page{Slug: "uses", Title: "Uses", Body: "b"}
	if err := store.CreatePage(ctx, pg); err != nil {
		t.Fatalf("CreatePage: %v", err)
	}

	if _, err := store.PageByID(ctx, pg.ID, content.Public); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("public should not reach a draft page by id, got %v", err)
	}
	if _, err := store.PageByID(ctx, pg.ID, content.Author); err != nil {
		t.Errorf("PageByID as author: %v", err)
	}
}

// Search is navigation, not discovery: a partial word has to match, which is
// the reason it is a substring match and not full text search.
func TestFindPostsMatchesTitleAndSlugBySubstring(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for _, p := range []*content.Post{
		{Slug: "running-kubernetes-at-home", Title: "Running Kubernetes at home", Body: "b"},
		{Slug: "why-go", Title: "Why Go", Body: "mentions kubernetes in the body"},
		{Slug: "k8s-notes", Title: "Notes", Body: "b"},
	} {
		if err := store.CreatePost(ctx, p); err != nil {
			t.Fatalf("CreatePost(%s): %v", p.Slug, err)
		}
	}

	tests := []struct {
		text string
		want []string
	}{
		{"kubern", []string{"running-kubernetes-at-home"}},
		{"KUBERNETES", []string{"running-kubernetes-at-home"}},
		{"k8s", []string{"k8s-notes"}},
		{"", []string{"k8s-notes", "running-kubernetes-at-home", "why-go"}},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			got, err := store.FindPosts(ctx, content.Author, content.Listing{Text: tt.text})
			if err != nil {
				t.Fatalf("FindPosts: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", slugs(got), tt.want)
			}
			for _, want := range tt.want {
				if !contains(slugs(got), want) {
					t.Errorf("got %v, want it to contain %q", slugs(got), want)
				}
			}
		})
	}
}

// A % typed into the search box is a character, not a wildcard.
func TestSearchTreatsWildcardsAsText(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	if err := store.CreatePost(ctx, newPost("ordinary", nil)); err != nil {
		t.Fatalf("CreatePost: %v", err)
	}

	got, err := store.FindPosts(ctx, content.Author, content.Listing{Text: "%"})
	if err != nil {
		t.Fatalf("FindPosts: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("a literal %% matched %v", slugs(got))
	}
}

// The Admin list is the reason FindPosts takes an Audience: it is the one
// caller that sees everything.
func TestFindPostsShowsDraftsFirstToTheAuthor(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for slug, published := range map[string]*time.Time{
		"published": at(-time.Hour),
		"scheduled": at(time.Hour),
		"draft":     nil,
	} {
		if err := store.CreatePost(ctx, newPost(slug, published)); err != nil {
			t.Fatalf("CreatePost(%s): %v", slug, err)
		}
	}

	got, err := store.FindPosts(ctx, content.Author, content.Listing{})
	if err != nil {
		t.Fatalf("FindPosts: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("author should see everything, got %v", slugs(got))
	}
	if got[0].Slug != "draft" {
		t.Errorf("drafts should sort first, got %v", slugs(got))
	}

	public, err := store.FindPosts(ctx, content.Public, content.Listing{})
	if err != nil {
		t.Fatalf("FindPosts(public): %v", err)
	}
	if len(public) != 1 || public[0].Slug != "published" {
		t.Errorf("public should see only the published post, got %v", slugs(public))
	}
}

// Without FindPages a Page could be written and never found again.
func TestFindPagesListsBySlug(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for _, slug := range []string{"uses", "about", "now"} {
		pg := &content.Page{Slug: slug, Title: "Title of " + slug, Body: "b"}
		if err := store.CreatePage(ctx, pg); err != nil {
			t.Fatalf("CreatePage(%s): %v", slug, err)
		}
	}

	got, err := store.FindPages(ctx, content.Author, content.Listing{})
	if err != nil {
		t.Fatalf("FindPages: %v", err)
	}
	want := []string{"about", "now", "uses"}
	for i := range want {
		if got[i].Slug != want[i] {
			t.Fatalf("pages = %v, want %v", pageSlugs(got), want)
		}
	}
}

// The Admin list asks for one more row than it shows, and that is how it knows
// there is another page without a count query.
func TestListingPaginates(t *testing.T) {
	ctx := context.Background()
	store := newStore(t)

	for i, slug := range []string{"first", "second", "third"} {
		if err := store.CreatePost(ctx, newPost(slug, at(-time.Duration(3-i)*time.Hour))); err != nil {
			t.Fatalf("CreatePost(%s): %v", slug, err)
		}
	}

	page1, err := store.FindPosts(ctx, content.Author, content.Listing{Page: 1, PerPage: 2})
	if err != nil {
		t.Fatalf("FindPosts: %v", err)
	}
	page2, err := store.FindPosts(ctx, content.Author, content.Listing{Page: 2, PerPage: 2})
	if err != nil {
		t.Fatalf("FindPosts: %v", err)
	}
	if got := slugs(page1); len(got) != 2 || got[0] != "third" {
		t.Errorf("page 1 = %v", got)
	}
	if got := slugs(page2); len(got) != 1 || got[0] != "first" {
		t.Errorf("page 2 = %v", got)
	}
}

func pageSlugs(pages []content.Page) []string {
	out := make([]string, len(pages))
	for i, p := range pages {
		out[i] = p.Slug
	}
	return out
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
