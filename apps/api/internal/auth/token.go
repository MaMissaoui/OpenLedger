package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalidToken is returned when an access token is missing, malformed,
// expired, or signed with the wrong key.
var ErrInvalidToken = errors.New("auth: invalid token")

// Claims is the authenticated principal carried by an access token.
type Claims struct {
	UserID string
	OrgID  string
}

// Issuer mints and verifies HS256 JWT access tokens and generates opaque
// refresh tokens. A single Issuer is safe for concurrent use.
type Issuer struct {
	secret    []byte
	accessTTL time.Duration
}

// NewIssuer builds an Issuer signing access tokens with secret, valid for
// accessTTL.
func NewIssuer(secret string, accessTTL time.Duration) *Issuer {
	return &Issuer{secret: []byte(secret), accessTTL: accessTTL}
}

// jwtClaims is the on-the-wire JWT claim set. Sub is the user ID; org is a
// private claim for the organization.
type jwtClaims struct {
	Org string `json:"org"`
	jwt.RegisteredClaims
}

// IssueAccess returns a signed access token for the given user and org.
func (i *Issuer) IssueAccess(userID, orgID string) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		Org: orgID,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(i.accessTTL)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(i.secret)
	if err != nil {
		return "", fmt.Errorf("auth: sign token: %w", err)
	}
	return tok, nil
}

// ParseAccess validates a signed access token and returns its claims, or
// ErrInvalidToken. It pins the signing method to HS256 to prevent algorithm
// confusion (e.g. "alg: none" or RS256 with the public key).
func (i *Issuer) ParseAccess(token string) (Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &jwtClaims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("%w: unexpected signing method %v", ErrInvalidToken, t.Header["alg"])
		}
		return i.secret, nil
	})
	if err != nil || !parsed.Valid {
		return Claims{}, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || claims.Subject == "" {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: claims.Subject, OrgID: claims.Org}, nil
}

// NewRefreshToken returns a new opaque refresh token (base64url of 32 random
// bytes) and its SHA-256 hash (hex). Only the hash is stored; the raw token is
// returned to the client once and never persisted.
func NewRefreshToken() (raw, hash string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("auth: read refresh token: %w", err)
	}
	raw = base64.RawURLEncoding.EncodeToString(b)
	return raw, HashRefreshToken(raw), nil
}

// HashRefreshToken returns the hex SHA-256 of a raw refresh token, used to look
// up its stored row without keeping the token itself.
func HashRefreshToken(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
