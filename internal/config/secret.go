package config

import "fmt"

// Secret is a config value that never renders itself.
//
// It is the DSN's redaction (ADR-0005) applied to the two values ADR-0009 keeps
// in config: the Author's password hash and the key their session cookie is
// signed with. Redacting in the type rather than in `config show` is what stops
// either from reaching a log line or an error message through a %v added later.
type Secret string

// Raw returns the value. It is the only way to obtain one, and the call site
// says so.
func (s Secret) Raw() string { return string(s) }

// Set reports whether a value was supplied at all. Callers ask this rather than
// comparing against "", so that no code path has to touch the value to find out
// it is absent.
func (s Secret) Set() bool { return s != "" }

// String satisfies fmt.Stringer, so %s and %v redact by default.
func (s Secret) String() string {
	if s == "" {
		return ""
	}
	return redacted
}

// MarshalYAML renders the redacted form for `config show`.
func (s Secret) MarshalYAML() (any, error) { return s.String(), nil }

var _ fmt.Stringer = Secret("")
