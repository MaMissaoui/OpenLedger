package auth

import (
	"errors"
	"testing"
	"time"
)

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if err := VerifyPassword("correct horse battery staple", hash); err != nil {
		t.Errorf("verify correct password: %v", err)
	}
	if err := VerifyPassword("wrong password", hash); !errors.Is(err, ErrMismatchedHash) {
		t.Errorf("verify wrong password: got %v, want ErrMismatchedHash", err)
	}
}

func TestHashPasswordSaltsAreUnique(t *testing.T) {
	a, _ := HashPassword("same")
	b, _ := HashPassword("same")
	if a == b {
		t.Error("expected different hashes for the same password (random salt)")
	}
}

func TestVerifyPasswordRejectsMalformedHash(t *testing.T) {
	if err := VerifyPassword("x", "not-a-phc-string"); err == nil || errors.Is(err, ErrMismatchedHash) {
		t.Errorf("got %v, want a format error", err)
	}
}

func TestIssueAndParseAccessToken(t *testing.T) {
	iss := NewIssuer("secret", time.Minute)
	tok, err := iss.IssueAccess("user-1", "org-1")
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	claims, err := iss.ParseAccess(tok)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.UserID != "user-1" || claims.OrgID != "org-1" {
		t.Errorf("claims = %+v, want user-1/org-1", claims)
	}
}

func TestParseAccessRejectsWrongSecret(t *testing.T) {
	tok, _ := NewIssuer("secret-a", time.Minute).IssueAccess("user-1", "org-1")
	if _, err := NewIssuer("secret-b", time.Minute).ParseAccess(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken", err)
	}
}

func TestParseAccessRejectsExpiredToken(t *testing.T) {
	iss := NewIssuer("secret", -time.Minute) // already expired
	tok, _ := iss.IssueAccess("user-1", "org-1")
	if _, err := iss.ParseAccess(tok); !errors.Is(err, ErrInvalidToken) {
		t.Errorf("got %v, want ErrInvalidToken for expired token", err)
	}
}

func TestRefreshTokenHashIsDeterministic(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatalf("new refresh: %v", err)
	}
	if raw == hash {
		t.Error("raw token must not equal its hash")
	}
	if HashRefreshToken(raw) != hash {
		t.Error("HashRefreshToken should reproduce the stored hash")
	}
}
