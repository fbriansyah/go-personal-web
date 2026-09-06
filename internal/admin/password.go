package admin

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// errBadHash means the configured hash is not something this code can verify
// against. It is a deployment mistake, not a failed login, and is kept distinct
// so that it can be reported once at startup instead of looking like a wrong
// password forever.
var errBadHash = errors.New("admin: password hash is not a valid argon2id encoding")

// argon2Params are the parameters a hash is *created* with.
//
// They are never used to verify one. A hash encodes the parameters it was made
// with, and verification reads them back out of it — which is the only way that
// works, since changing these values must not invalidate hashes made before the
// change. Getting this backwards is a popular bug: it fails for every password,
// including the right one, and looks like the password is wrong.
var argon2Params = struct {
	time    uint32
	memory  uint32
	threads uint8
	keyLen  uint32
	saltLen int
}{time: 3, memory: 64 * 1024, threads: 4, keyLen: 32, saltLen: 16}

// HashPassword returns the argon2id encoding of password, in the format
// `personal-web admin hash-password` prints and PW_ADMIN_PASSWORD_HASH holds.
func HashPassword(password string) (string, error) {
	salt := make([]byte, argon2Params.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("admin: generating salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt,
		argon2Params.time, argon2Params.memory, argon2Params.threads, argon2Params.keyLen)

	b64 := base64.RawStdEncoding
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argon2Params.memory, argon2Params.time, argon2Params.threads,
		b64.EncodeToString(salt), b64.EncodeToString(key)), nil
}

// verifyPassword reports whether password matches the encoded hash.
//
// The comparison is constant-time, and every way of failing — wrong password,
// wrong length, unparseable hash — is reported as a plain false to the caller
// that decides what to say, so nothing about the difference reaches a response.
func verifyPassword(encoded, password string) (bool, error) {
	h, err := parseHash(encoded)
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(password), h.salt, h.time, h.memory, h.threads, uint32(len(h.key)))
	return subtle.ConstantTimeCompare(got, h.key) == 1, nil
}

type argon2Hash struct {
	memory, time uint32
	threads      uint8
	salt, key    []byte
}

// parseHash reads the parameters back out of an encoded hash. Everything the
// verification needs comes from here and nothing from config.
func parseHash(encoded string) (argon2Hash, error) {
	var h argon2Hash

	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, key
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return h, errBadHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return h, errBadHash
	}
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &h.memory, &h.time, &h.threads); err != nil {
		return h, errBadHash
	}

	b64 := base64.RawStdEncoding
	salt, err := b64.DecodeString(parts[4])
	if err != nil {
		return h, errBadHash
	}
	key, err := b64.DecodeString(parts[5])
	if err != nil || len(key) == 0 {
		return h, errBadHash
	}
	h.salt, h.key = salt, key
	return h, nil
}
