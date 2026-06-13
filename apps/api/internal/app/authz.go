package app

import (
	"context"
	"errors"
)

// ErrForbidden is returned when an authenticated user has no membership on the
// book an operation touches. Handlers map it to HTTP 403.
var ErrForbidden = errors.New("forbidden")

// ErrInsufficientRole is returned when the user has a membership on the book but
// their role does not permit the requested action (e.g. a viewer posting a
// transaction). Handlers map it to HTTP 403.
var ErrInsufficientRole = errors.New("insufficient role")

// Role is a user's permission level on a book. Roles are ranked: a higher role
// includes every capability of the lower ones. The set mirrors the CHECK
// constraint on the memberships table.
type Role string

// The ranked set of roles, lowest to highest capability.
const (
	RoleViewer Role = "viewer" // read-only
	RoleEditor Role = "editor" // read + post/edit ledger data
	RoleAdmin  Role = "admin"  // editor + (future) book administration
	RoleOwner  Role = "owner"  // full control
)

// rank orders roles for capability comparison. An unknown role ranks 0, so it
// grants nothing.
func (r Role) rank() int {
	switch r {
	case RoleViewer:
		return 1
	case RoleEditor:
		return 2
	case RoleAdmin:
		return 3
	case RoleOwner:
		return 4
	default:
		return 0
	}
}

// Access is the capability an operation requires. Handlers express intent
// (read vs. write) and the policy here maps it to the minimum role.
type Access int

// The access levels handlers can require.
const (
	AccessRead  Access = iota + 1 // view accounts and registers
	AccessWrite                   // create accounts, post transactions
)

// minRole returns the lowest role that satisfies the access level.
func (a Access) minRole() Role {
	if a == AccessWrite {
		return RoleEditor
	}
	return RoleViewer
}

// permits reports whether the role is allowed the requested access.
func (r Role) permits(need Access) bool {
	return r.rank() >= need.minRole().rank()
}

// MembershipRepository reads the book-membership facts authorization needs.
type MembershipRepository interface {
	// UserBookRole returns the user's role on the book and whether a membership
	// exists at all (false means no membership row).
	UserBookRole(ctx context.Context, userID, bookGUID string) (Role, bool, error)
	// BookGUIDForAccount returns the book an account belongs to (by walking up
	// to the root account), or ErrAccountNotFound if the account does not exist.
	BookGUIDForAccount(ctx context.Context, accountGUID string) (string, error)
}

// AuthzService answers "may this user perform this action on this book?" for the
// HTTP layer. It is the single place per-book access is decided, so every
// book-scoped route enforces roles the same way.
type AuthzService struct {
	repo MembershipRepository
}

// NewAuthzService builds an AuthzService backed by repo.
func NewAuthzService(repo MembershipRepository) *AuthzService {
	return &AuthzService{repo: repo}
}

// AuthorizeBook returns nil if the user may perform a need-level action on
// bookGUID, ErrForbidden if the user has no membership, ErrInsufficientRole if
// their role is too low, or a wrapped repository error.
func (s *AuthzService) AuthorizeBook(ctx context.Context, userID, bookGUID string, need Access) error {
	role, ok, err := s.repo.UserBookRole(ctx, userID, bookGUID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrForbidden
	}
	if !role.permits(need) {
		return ErrInsufficientRole
	}
	return nil
}

// AuthorizeAccount resolves the account's book and authorizes the user against
// it, returning ErrAccountNotFound if the account does not exist.
func (s *AuthzService) AuthorizeAccount(ctx context.Context, userID, accountGUID string, need Access) error {
	bookGUID, err := s.repo.BookGUIDForAccount(ctx, accountGUID)
	if err != nil {
		return err
	}
	return s.AuthorizeBook(ctx, userID, bookGUID, need)
}

// AuthorizeAccounts authorizes the user against the book(s) the given accounts
// belong to. Every account must resolve to a book the user can access at the
// need level; the first failure (ErrForbidden, ErrInsufficientRole, or
// ErrAccountNotFound) stops the check. Books are de-duplicated so a multi-split
// transaction in one book costs one membership lookup.
func (s *AuthzService) AuthorizeAccounts(ctx context.Context, userID string, accountGUIDs []string, need Access) error {
	checked := make(map[string]struct{})
	for _, accountGUID := range accountGUIDs {
		bookGUID, err := s.repo.BookGUIDForAccount(ctx, accountGUID)
		if err != nil {
			return err
		}
		if _, done := checked[bookGUID]; done {
			continue
		}
		checked[bookGUID] = struct{}{}
		if err := s.AuthorizeBook(ctx, userID, bookGUID, need); err != nil {
			return err
		}
	}
	return nil
}
