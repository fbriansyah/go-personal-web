package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestDSNRedactsPassword(t *testing.T) {
	tests := []struct {
		name string
		dsn  DSN
		want string
	}{
		{
			name: "password is replaced",
			dsn:  "postgres://writer:hunter2@db.example.com:5432/pw?sslmode=require",
			want: "postgres://writer:xxxxx@db.example.com:5432/pw?sslmode=require",
		},
		{
			name: "user without a password is untouched",
			dsn:  "postgres://writer@db.example.com:5432/pw",
			want: "postgres://writer@db.example.com:5432/pw",
		},
		{
			name: "no credentials at all",
			dsn:  "postgres://db.example.com:5432/pw",
			want: "postgres://db.example.com:5432/pw",
		},
		{
			name: "empty stays empty",
			dsn:  "",
			want: "",
		},
		{
			// An unparseable DSN may still contain a password, so none of it is
			// shown rather than guessing which part is safe.
			name: "unparseable reveals nothing",
			dsn:  DSN("postgres://writer:hunter2@db.example.com:5432/pw\x7f"),
			want: "<unparseable dsn>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.dsn.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}

			got, err := tt.dsn.MarshalYAML()
			if err != nil {
				t.Fatalf("MarshalYAML: %v", err)
			}
			if got != tt.want {
				t.Errorf("MarshalYAML() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The point of the type is that redaction cannot be bypassed by accident, so
// the default verbs must not print the password either.
func TestDSNRedactsUnderFormatVerbs(t *testing.T) {
	const password = "hunter2"
	dsn := DSN("postgres://writer:" + password + "@db.example.com:5432/pw")

	for _, format := range []string{"%v", "%s", "%q"} {
		if got := fmt.Sprintf(format, dsn); strings.Contains(got, password) {
			t.Errorf("%s printed the password: %s", format, got)
		}
	}

	if raw := dsn.Raw(); !strings.Contains(raw, password) {
		t.Errorf("Raw() must keep the password, got %q", raw)
	}
}

func TestDSNValidate(t *testing.T) {
	tests := []struct {
		name    string
		dsn     DSN
		wantErr bool
	}{
		{"the development default", DSN(DevDatabaseURL), false},
		{"postgresql scheme", "postgresql://u:p@host:5432/db", false},
		{"empty", "", true},
		{"wrong scheme", "mysql://u:p@host:3306/db", true},
		{"no host", "postgres:///db", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.dsn.validate()
			if (err != nil) != tt.wantErr {
				t.Fatalf("validate() error = %v, wantErr %v", err, tt.wantErr)
			}
			// A validation error is shown to the user, so it must not quote the
			// DSN back at them.
			if err != nil && strings.Contains(err.Error(), "p@host") {
				t.Errorf("error leaked the DSN: %v", err)
			}
		})
	}
}
