package config

import (
	"fmt"
	"strings"
	"testing"
)

// A Secret must not render itself, whatever it is asked with. The redaction is
// in the type rather than in `config show` so that it survives a %v added to a
// log line later (ADR-0005, applied to ADR-0009's two values).
func TestSecretNeverRendersItself(t *testing.T) {
	const value = "$argon2id$v=19$m=65536,t=3,p=4$c2FsdA$aGFzaA"
	s := Secret(value)

	for name, got := range map[string]string{
		"String":          s.String(),
		"%s":              fmt.Sprintf("%s", s),
		"%v":              fmt.Sprintf("%v", s),
		"inside a struct": fmt.Sprintf("%v", AdminConfig{PasswordHash: s}),
	} {
		if strings.Contains(got, "argon2id") || strings.Contains(got, value) {
			t.Errorf("%s leaked the secret: %q", name, got)
		}
		if name != "inside a struct" && got != redacted {
			t.Errorf("%s = %q, want %q", name, got, redacted)
		}
	}

	if s.Raw() != value {
		t.Error("Raw should return the value; it is the only way to obtain one")
	}
	if Secret("").String() != "" {
		t.Error("an unset secret should render as nothing, not as a redaction")
	}
}

// Both secrets arrive from the environment and nowhere else, which is the whole
// point of keeping the Author in config.
//
// This is a regression test with a specific bug behind it: viper only reads an
// environment variable for a key it already knows, so a key declared without a
// default is invisible however carefully the variable is set. Admin silently
// refused to mount, and the config looked correct.
func TestAdminSecretsAreReadFromTheEnvironment(t *testing.T) {
	t.Setenv("PW_ADMIN_PASSWORD_HASH", "a-hash")
	t.Setenv("PW_ADMIN_SESSION_SECRET", "a-key")

	cfg, err := Load(New(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Admin.PasswordHash.Raw() != "a-hash" {
		t.Errorf("password_hash = %q, want it read from the environment", cfg.Admin.PasswordHash.Raw())
	}
	if cfg.Admin.SessionSecret.Raw() != "a-key" {
		t.Errorf("session_secret = %q, want it read from the environment", cfg.Admin.SessionSecret.Raw())
	}
	if !cfg.Admin.Enabled() {
		t.Error("Admin should be enabled once both secrets are set")
	}
}

// Neither secret set is the normal case for a fresh clone: the public site
// serves and Admin is simply absent.
func TestAdminIsDisabledWithoutBothSecrets(t *testing.T) {
	cfg, err := Load(New(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Admin.Enabled() {
		t.Error("Admin should not be enabled without secrets")
	}
	if cfg.Admin.SessionTTL == 0 || cfg.Admin.Timezone == "" || cfg.Admin.LoginAttempts == 0 {
		t.Errorf("the rest of the admin defaults should still be there: %+v", cfg.Admin)
	}
}

// One secret without the other is a deployment that believes Admin is protected
// when it is only absent. Saying so is the reason config rejects what it
// cannot honour.
func TestOneAdminSecretWithoutTheOtherIsRejected(t *testing.T) {
	t.Setenv("PW_ADMIN_PASSWORD_HASH", "a-hash")

	_, err := Load(New(), "")
	if err == nil {
		t.Fatal("a password hash with no signing key was accepted")
	}
	if !strings.Contains(err.Error(), "must be set together") {
		t.Errorf("err = %v, want it to name the pair", err)
	}
}

func TestAnUnknownTimezoneIsRejected(t *testing.T) {
	t.Setenv("PW_ADMIN_TIMEZONE", "Mars/Olympus_Mons")

	if _, err := Load(New(), ""); err == nil {
		t.Fatal("an unknown zone was accepted")
	}
}

// The zone is stated so that moving a deployment cannot shift what a schedule
// means, which only works if it is actually resolved.
func TestTimezoneResolves(t *testing.T) {
	t.Setenv("PW_ADMIN_TIMEZONE", "Asia/Jakarta")

	cfg, err := Load(New(), "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := cfg.Admin.Location().String(); got != "Asia/Jakarta" {
		t.Errorf("Location = %q, want Asia/Jakarta", got)
	}
}
