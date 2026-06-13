package app

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/openledger/openledger/apps/api/internal/auth"
)

// Auth-related sentinel errors. Handlers map these to status codes.
var (
	ErrEmailTaken         = errors.New("email already registered")
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrInvalidRefresh     = errors.New("invalid or expired refresh token")
)

// UserCredentials is the minimal record needed to authenticate a login.
type UserCredentials struct {
	UserID       string
	OrgID        string
	PasswordHash string
}

// UserRepository persists and reads users, organizations, and refresh tokens.
type UserRepository interface {
	// CreateOrgAndUser creates an organization and its first user atomically.
	// It returns ErrEmailTaken if the email is already registered.
	CreateOrgAndUser(ctx context.Context, orgName, email, passwordHash string) (userID, orgID string, err error)
	// UserByEmail returns credentials for login, or ErrInvalidCredentials if no
	// such user exists (kept deliberately vague to avoid user enumeration).
	UserByEmail(ctx context.Context, email string) (UserCredentials, error)
	// StoreRefreshToken records a hashed refresh token for a user.
	StoreRefreshToken(ctx context.Context, userID, tokenHash string, expiresAt time.Time) error
	// RotateRefreshToken atomically revokes the token identified by oldHash (if
	// it is active and unexpired) and stores newHash, returning the owning user
	// and org. It returns ErrInvalidRefresh if oldHash is not currently valid.
	RotateRefreshToken(ctx context.Context, oldHash, newHash string, newExpiresAt time.Time) (userID, orgID string, err error)
	// RevokeRefreshToken marks a token revoked (logout). Revoking an unknown or
	// already-revoked token is not an error.
	RevokeRefreshToken(ctx context.Context, tokenHash string) error
}

// Tokens is the credential pair returned to a client after authentication.
type Tokens struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int // access-token lifetime in seconds
}

// AuthService handles registration, login, refresh, and logout.
type AuthService struct {
	repo       UserRepository
	issuer     *auth.Issuer
	accessTTL  time.Duration
	refreshTTL time.Duration
	now        func() time.Time
}

// NewAuthService builds an AuthService. accessTTL and refreshTTL set the
// lifetimes of access and refresh tokens respectively.
func NewAuthService(repo UserRepository, issuer *auth.Issuer, accessTTL, refreshTTL time.Duration) *AuthService {
	return &AuthService{repo: repo, issuer: issuer, accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}
}

// Register creates an organization and its first user, then issues tokens.
func (s *AuthService) Register(ctx context.Context, orgName, email, password string) (Tokens, error) {
	if email == "" || password == "" {
		return Tokens{}, fmt.Errorf("%w: email and password are required", ErrInvalidInput)
	}
	if len(password) < 8 {
		return Tokens{}, fmt.Errorf("%w: password must be at least 8 characters", ErrInvalidInput)
	}
	if orgName == "" {
		orgName = email
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return Tokens{}, err
	}
	userID, orgID, err := s.repo.CreateOrgAndUser(ctx, orgName, email, hash)
	if err != nil {
		return Tokens{}, err
	}
	return s.issueTokens(ctx, userID, orgID)
}

// Login verifies credentials and issues tokens, or returns
// ErrInvalidCredentials. Verification runs even for unknown users would be
// ideal to equalize timing; here UserByEmail already returns the vague error.
func (s *AuthService) Login(ctx context.Context, email, password string) (Tokens, error) {
	creds, err := s.repo.UserByEmail(ctx, email)
	if err != nil {
		return Tokens{}, err
	}
	if err := auth.VerifyPassword(password, creds.PasswordHash); err != nil {
		return Tokens{}, ErrInvalidCredentials
	}
	return s.issueTokens(ctx, creds.UserID, creds.OrgID)
}

// Refresh rotates a refresh token: the presented token is revoked and a new
// access/refresh pair is issued. Reusing a revoked token returns
// ErrInvalidRefresh.
func (s *AuthService) Refresh(ctx context.Context, rawRefresh string) (Tokens, error) {
	if rawRefresh == "" {
		return Tokens{}, ErrInvalidRefresh
	}
	newRaw, newHash, err := auth.NewRefreshToken()
	if err != nil {
		return Tokens{}, err
	}
	userID, orgID, err := s.repo.RotateRefreshToken(ctx,
		auth.HashRefreshToken(rawRefresh), newHash, s.now().Add(s.refreshTTL))
	if err != nil {
		return Tokens{}, err
	}
	access, err := s.issuer.IssueAccess(userID, orgID)
	if err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: newRaw, ExpiresIn: int(s.accessTTL.Seconds())}, nil
}

// Logout revokes a refresh token. It is idempotent.
func (s *AuthService) Logout(ctx context.Context, rawRefresh string) error {
	if rawRefresh == "" {
		return nil
	}
	return s.repo.RevokeRefreshToken(ctx, auth.HashRefreshToken(rawRefresh))
}

// ParseAccess validates a bearer access token and returns its claims.
func (s *AuthService) ParseAccess(token string) (auth.Claims, error) {
	return s.issuer.ParseAccess(token)
}

// issueTokens mints an access token and a fresh stored refresh token.
func (s *AuthService) issueTokens(ctx context.Context, userID, orgID string) (Tokens, error) {
	access, err := s.issuer.IssueAccess(userID, orgID)
	if err != nil {
		return Tokens{}, err
	}
	raw, hash, err := auth.NewRefreshToken()
	if err != nil {
		return Tokens{}, err
	}
	if err := s.repo.StoreRefreshToken(ctx, userID, hash, s.now().Add(s.refreshTTL)); err != nil {
		return Tokens{}, err
	}
	return Tokens{AccessToken: access, RefreshToken: raw, ExpiresIn: int(s.accessTTL.Seconds())}, nil
}
