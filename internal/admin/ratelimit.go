package admin

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// limiter counts attempts per address over a fixed window.
//
// It stands in front of argon2id rather than behind it. Argon2id is expensive
// by design — tens of megabytes and hundreds of milliseconds per attempt — so
// on a public endpoint it is a resource an unauthenticated caller would
// otherwise get to spend freely, and the safer the parameters the cheaper the
// denial of service. A limiter that counts after the hash has been computed
// protects nothing: the cost is already paid (ADR-0015).
//
// The counters live in memory and are lost on restart. That is not a gap: they
// defend the running process, and there is no session storage to put them in.
type limiter struct {
	limit  int
	window time.Duration
	now    func() time.Time

	mu      sync.Mutex
	windows map[string]*window
}

type window struct {
	count   int
	started time.Time
}

func newLimiter(limit int, per time.Duration) *limiter {
	return &limiter{limit: limit, window: per, now: time.Now, windows: map[string]*window{}}
}

// allow records an attempt by key and reports whether it may proceed.
func (l *limiter) allow(key string) bool {
	now := l.now()

	l.mu.Lock()
	defer l.mu.Unlock()

	// Expired windows are dropped on the way past, so a long-running process
	// does not accumulate an entry per address that ever tried once.
	for k, w := range l.windows {
		if now.Sub(w.started) >= l.window {
			delete(l.windows, k)
		}
	}

	w, ok := l.windows[key]
	if !ok {
		l.windows[key] = &window{count: 1, started: now}
		return true
	}
	w.count++
	return w.count <= l.limit
}

// reset forgets an address, called when it authenticates successfully so that a
// day of typos does not lock out the Author who then remembers the password.
func (l *limiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.windows, key)
}

// clientIP is the key attempts are counted under. RemoteAddr is used rather
// than a forwarded header: a header is set by whoever is calling unless a proxy
// is known to overwrite it, and trusting one here would let an attacker spread
// their attempts across as many keys as they like.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
