// Package auth holds authentication primitives: Argon2id password hashing and
// JWT access-token issuing/parsing. It has no DB or HTTP dependencies.
package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// ErrMismatchedHash is returned when a password does not match its stored hash.
var ErrMismatchedHash = errors.New("auth: password does not match")

// argon2Params are the Argon2id cost parameters. They are encoded into each
// hash so existing hashes stay verifiable if these defaults change later.
type argon2Params struct {
	memory  uint32 // KiB
	time    uint32
	threads uint8
	keyLen  uint32
	saltLen uint32
}

var defaultParams = argon2Params{memory: 64 * 1024, time: 1, threads: 4, keyLen: 32, saltLen: 16}

// HashPassword returns a PHC-formatted Argon2id hash of password, with a random
// salt and the default cost parameters embedded.
func HashPassword(password string) (string, error) {
	p := defaultParams
	salt := make([]byte, p.saltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("auth: read salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, p.memory, p.time, p.threads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key),
	), nil
}

// VerifyPassword reports nil if password matches the PHC-formatted Argon2id
// hash, ErrMismatchedHash if it does not, or an error if the hash is malformed.
// The comparison is constant-time.
func VerifyPassword(password, encoded string) error {
	p, salt, key, err := decodeHash(encoded)
	if err != nil {
		return err
	}
	other := argon2.IDKey([]byte(password), salt, p.time, p.memory, p.threads, p.keyLen)
	if subtle.ConstantTimeCompare(key, other) == 1 {
		return nil
	}
	return ErrMismatchedHash
}

func decodeHash(encoded string) (argon2Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return argon2Params{}, nil, nil, errors.New("auth: invalid hash format")
	}
	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("auth: parse version: %w", err)
	}
	if version != argon2.Version {
		return argon2Params{}, nil, nil, fmt.Errorf("auth: unsupported argon2 version %d", version)
	}
	var p argon2Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.memory, &p.time, &p.threads); err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("auth: parse params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("auth: decode salt: %w", err)
	}
	key, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return argon2Params{}, nil, nil, fmt.Errorf("auth: decode key: %w", err)
	}
	p.keyLen = uint32(len(key))
	p.saltLen = uint32(len(salt))
	return p, salt, key, nil
}
