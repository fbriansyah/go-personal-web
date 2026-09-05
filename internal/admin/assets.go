package admin

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// static holds the front end.
//
// htmx and the stylesheet are files in this repository, embedded in the binary
// and served from here — not fetched from a CDN (ADR-0013). A project that
// refuses to read its migrations from disk should not fetch its JavaScript from
// someone else's machine, and the stakes are higher here: a third-party script
// on /admin runs on the page holding the session cookie.
//
// The htmx filename carries its version, so `git log` says which one is
// deployed without anyone opening the file.
//
//go:embed static
var static embed.FS

// htmxFile is the vendored htmx, named here so that updating it is one commit
// that changes a filename and this line.
const htmxFile = "htmx-2.0.10.min.js"

// contentSecurityPolicy is affordable precisely because the assets are ours.
//
// It is what rules out inline <style> and inline hx-on: handlers, and therefore
// templ's css expressions (ADR-0010). htmx's own configuration travels in a
// <meta> tag, which is not a script and needs no exception here.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self'; " +
	"style-src 'self'; " +
	"img-src 'self' data:; " +
	"form-action 'self'; " +
	"frame-ancestors 'none'; " +
	"base-uri 'none'"

// serveAsset serves the embedded front end.
//
// Only two files are reachable, by name. Serving the directory would make every
// future file in it public by default, which is the wrong way round.
func serveAsset(w http.ResponseWriter, r *http.Request) {
	var name string
	switch strings.TrimPrefix(r.URL.Path, "/admin/static/") {
	case "htmx.min.js":
		name = htmxFile
	case "app.css":
		name = "app.css"
	default:
		http.NotFound(w, r)
		return
	}

	body, err := fs.ReadFile(static, "static/"+name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if strings.HasSuffix(name, ".css") {
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
	}
	// The URL is stable while the file behind it is not, so the cache has to be
	// revalidated. These are two small files served by the same process; there
	// is nothing here worth a fingerprinted path.
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = w.Write(body)
}

// secured sets the headers every Admin response carries.
func secured(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}
