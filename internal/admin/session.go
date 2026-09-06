package admin

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// cookieName is the session cookie. It is scoped to /admin, so nothing the
// public site serves ever carries it.
const cookieName = "pw_session"

// errNoSession covers every way a request can fail to carry a valid session:
// no cookie, a forged signature, a malformed payload, an expiry that has
// passed. They are deliberately indistinguishable — telling them apart tells an
// attacker how close they are (ADR-0015).
var errNoSession = errors.New("admin: no session")

// session is what the signed cookie carries. There is one Author, so there is
// no identity to record: the only thing worth signing is when this session was
// issued and when it dies.
type session struct {
	issuedAt  time.Time
	expiresAt time.Time
}

// sessions issues and verifies session cookies.
type sessions struct {
	key      []byte
	ttl      time.Duration
	insecure bool
	now      func() time.Time
}

func newSessions(secret string, ttl time.Duration, insecure bool) *sessions {
	return &sessions{key: []byte(secret), ttl: ttl, insecure: insecure, now: time.Now}
}

// issue returns the cookie for a session starting now.
//
// The expiry is inside the signed payload as well as in Max-Age. Max-Age is an
// instruction to the browser, and anyone holding a copy of the cookie value can
// ignore it forever; the signed copy is the one the server enforces, and it is
// what gives a stolen cookie a life (ADR-0015).
func (s *sessions) issue() *http.Cookie {
	now := s.now()
	value := s.sign(session{issuedAt: now, expiresAt: now.Add(s.ttl)})

	return &http.Cookie{
		Name:  cookieName,
		Value: value,
		Path:  "/admin",
		// Strict is the whole of the CSRF defence: the browser withholds this
		// cookie from any cross-site request, including a POST (ADR-0015).
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
		Secure:   !s.insecure,
		MaxAge:   int(s.ttl.Seconds()),
	}
}

// clear returns the cookie that removes the session from this browser.
//
// It is all logging out can do. With no session storage there is nothing to
// revoke, so a copy of the cookie taken elsewhere stays valid until it expires;
// rotating the signing secret is the immediate revocation (ADR-0009).
func (s *sessions) clear() *http.Cookie {
	return &http.Cookie{
		Name:     cookieName,
		Value:    "",
		Path:     "/admin",
		SameSite: http.SameSiteStrictMode,
		HttpOnly: true,
		Secure:   !s.insecure,
		MaxAge:   -1,
	}
}

// verify returns the session carried by r, or errNoSession.
func (s *sessions) verify(r *http.Request) (session, error) {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return session{}, errNoSession
	}
	return s.open(c.Value)
}

// sign renders a session as `payload.signature`, both base64url.
func (s *sessions) sign(sess session) string {
	payload := fmt.Sprintf("1|%d|%d", sess.issuedAt.Unix(), sess.expiresAt.Unix())
	enc := base64.RawURLEncoding
	return enc.EncodeToString([]byte(payload)) + "." + enc.EncodeToString(s.mac(payload))
}

// open verifies a cookie value and returns the session it carries.
func (s *sessions) open(value string) (session, error) {
	encPayload, encMAC, ok := strings.Cut(value, ".")
	if !ok {
		return session{}, errNoSession
	}
	enc := base64.RawURLEncoding
	payload, err := enc.DecodeString(encPayload)
	if err != nil {
		return session{}, errNoSession
	}
	mac, err := enc.DecodeString(encMAC)
	if err != nil {
		return session{}, errNoSession
	}
	// The signature is checked before the payload is believed, so a forged
	// payload never reaches the parsing below.
	if !hmac.Equal(mac, s.mac(string(payload))) {
		return session{}, errNoSession
	}

	fields := strings.Split(string(payload), "|")
	if len(fields) != 3 || fields[0] != "1" {
		return session{}, errNoSession
	}
	issued, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil {
		return session{}, errNoSession
	}
	expires, err := strconv.ParseInt(fields[2], 10, 64)
	if err != nil {
		return session{}, errNoSession
	}

	sess := session{issuedAt: time.Unix(issued, 0), expiresAt: time.Unix(expires, 0)}
	if !s.now().Before(sess.expiresAt) {
		return session{}, errNoSession
	}
	return sess, nil
}

func (s *sessions) mac(payload string) []byte {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte(payload))
	return m.Sum(nil)
}
