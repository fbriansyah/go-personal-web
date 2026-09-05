# Sessions expire on a signed deadline, and the login form is rate limited

ADR-0009 keeps the Author's credential in config and issues an HMAC-signed
cookie with no session storage. This settles what that cookie contains and what
guards the endpoint that issues it.

The payload is an issue time and an **expiry, both signed**. Expiry cannot live
in `Max-Age` alone: that is an instruction to the browser, and anyone holding a
copy of the cookie value can ignore it forever. Signed, the server enforces it
and a stolen cookie has a life. Seven days, absolute, with no renewal.

Sliding expiry was rejected, and the reason matters more than the choice.
Without session storage there is nothing to revoke against, so a sliding window
is extended by the attacker's own use of the cookie and never closes. The
property worth having is the one that can be stated plainly: seven days after it
was issued, that cookie is dead. Rotating the signing secret remains the immediate
revocation, as ADR-0009 says — the key is `admin.session_secret`, so the
variable is `PW_ADMIN_SESSION_SECRET`, not the `PW_SESSION_SECRET` ADR-0009
wrote before the key existed.

The cookie is `HttpOnly`, `SameSite=Strict`, `Path=/admin`, and `Secure` unless
config explicitly says otherwise. `Strict` is the whole of the CSRF defence: the
browser withholds the cookie from any cross-site request, including a POST, so a
token would guard a door already shut — at the price of threading it through
every form component and every handler. The cost of `Strict` is that following
a link to `/admin` from outside lands on the login page; the only person that
happens to is the Author, on a surface nothing links to.

`Secure` is disabled by an explicit config key, never by detecting localhost.
Magic detection is how a production deployment quietly serves an insecure
cookie.

The login form is rate limited per IP, in memory, **in front of** argon2id.
The threat is not guessing: argon2id at sane parameters burns tens of megabytes
and hundreds of milliseconds per attempt, which on a public endpoint is a
resource the attacker gets to spend on the Author's behalf. The safer the
parameters, the cheaper the denial of service. A limiter behind the hash
protects nothing, since the cost is already paid. Losing the counters on restart
is fine: they defend the running process.

A global lockout after N failures was rejected — it hands anyone on the internet
a button that locks the Author out of their own site.

## Consequences

- A bad password, an expired cookie, a forged signature and a malformed cookie
  are all answered identically: no session. Distinguishing them tells an
  attacker how close they are.
- argon2id parameters are read from the stored hash at verification, never from
  config. A hash encodes the parameters it was made with; verifying against
  config values would fail silently the day config changes.
- Absolute expiry means a session can lapse mid-sentence. ADR-0014 is the
  answer to that.
