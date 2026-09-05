package config

import (
	"fmt"
	"net/url"
)

// redacted replaces the password of a DSN wherever it is rendered.
const redacted = "xxxxx"

// DSN is a database connection string that never renders its password.
//
// The redaction lives in the type rather than in `config show`, so that a DSN
// cannot leak through a log line, an error message, or a %v added later. Code
// that actually needs to connect asks for it by name, via Raw.
type DSN string

// Raw returns the connection string with its password intact. It is the only
// way to obtain one, and the call site says so.
func (d DSN) Raw() string { return string(d) }

// String renders the DSN with its password replaced. It satisfies fmt.Stringer,
// so %s and %v redact by default.
func (d DSN) String() string {
	if d == "" {
		return ""
	}
	u, err := url.Parse(string(d))
	if err != nil {
		// An unparseable DSN may still contain a password, so nothing of it is
		// shown. Validate rejects this case before it can reach a user.
		return "<unparseable dsn>"
	}
	if _, hasPassword := u.User.Password(); hasPassword {
		u.User = url.UserPassword(u.User.Username(), redacted)
	}
	return u.String()
}

// MarshalYAML renders the redacted form for `config show`.
func (d DSN) MarshalYAML() (any, error) { return d.String(), nil }

// validate reports whether the DSN is a usable PostgreSQL connection string.
func (d DSN) validate() error {
	if d == "" {
		return fmt.Errorf("database.url must not be empty")
	}
	u, err := url.Parse(string(d))
	if err != nil {
		// The error from url.Parse quotes the input, password and all.
		return fmt.Errorf("database.url is not a valid URL")
	}
	switch u.Scheme {
	case "postgres", "postgresql":
	default:
		return fmt.Errorf("database.url must use the postgres:// scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("database.url must name a host")
	}
	return nil
}
