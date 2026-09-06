package admin

import (
	"net/http"
	"strconv"

	"github.com/fbriansyah/go-personal-web/internal/content"
	"github.com/fbriansyah/go-personal-web/internal/view"
)

// listPosts serves the whole screen, the list after a search, and the next page
// of rows — from one URL.
//
// Which of the three comes back is decided by the request rather than by a
// second endpoint, so the search box, the "load more" button and a cold reload
// all share the same address and cannot disagree about the filter.
func (h *handler) listPosts(w http.ResponseWriter, r *http.Request) {
	l := listingFrom(r)

	// One row more than is shown. That extra row is how the button knows there
	// is another page, with no count query anywhere.
	posts, err := h.store.FindPosts(r.Context(), content.Author, l)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	posts, next := trim(posts, l)

	listing := view.PostListing{
		Listing: view.Listing{Query: l.Text, NextPage: next, Zone: h.zone},
		Posts:   posts,
	}
	now := h.now()

	h.render(w, r, screen{
		title:    "Posts",
		nav:      view.NavPosts,
		body:     view.Posts(listing, now),
		fragment: pickFragment(l.Page, view.PostRegion(listing, now), view.PostRows(listing, now)),
	})
}

func (h *handler) listPages(w http.ResponseWriter, r *http.Request) {
	l := listingFrom(r)

	pages, err := h.store.FindPages(r.Context(), content.Author, l)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pages, next := trim(pages, l)

	listing := view.PageListing{
		Listing: view.Listing{Query: l.Text, NextPage: next, Zone: h.zone},
		Pages:   pages,
	}
	now := h.now()

	h.render(w, r, screen{
		title:    "Pages",
		nav:      view.NavPages,
		body:     view.Pages(listing, now),
		fragment: pickFragment(l.Page, view.PageRegion(listing, now), view.PageRows(listing, now)),
	})
}

// listingFrom reads the filter and the page out of the query string. Both are
// in the URL because a search result has to survive a reload (ADR-0011).
func listingFrom(r *http.Request) content.Listing {
	page, err := strconv.Atoi(r.URL.Query().Get("page"))
	if err != nil || page < 1 {
		page = 1
	}
	return content.Listing{
		Text: r.URL.Query().Get("q"),
		Page: page,
		// One more than is shown, to discover whether there is a page after
		// this one.
		PerPage: perPage + 1,
	}
}

// trim drops the extra row and reports the page after this one, or 0.
func trim[T any](rows []T, l content.Listing) ([]T, int) {
	if len(rows) <= perPage {
		return rows, 0
	}
	return rows[:perPage], l.Page + 1
}

// pickFragment chooses what an htmx request gets: the first page replaces the
// whole list, because it is a search; a later page is rows to append.
func pickFragment[T any](page int, region, rows T) T {
	if page > 1 {
		return rows
	}
	return region
}
