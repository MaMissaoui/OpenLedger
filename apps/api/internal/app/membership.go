package app

import (
	"context"
	"errors"
)

// ErrUserNotFound is returned when a member operation references a user (by
// email) who has never signed in, so no users row exists. Handlers map it to
// HTTP 404.
var ErrUserNotFound = errors.New("user not found")

// ErrLastOwner is returned when an operation would leave a book with no owner
// (removing or demoting its only owner). A book must always have at least one
// owner, so handlers map this to HTTP 409.
var ErrLastOwner = errors.New("a book must keep at least one owner")

// Member is a user's place on a book: who they are (by email / LDAP uid) and
// what they may do (their role). It is the read model behind the Settings
// members screen.
type Member struct {
	UserID   string
	Email    string
	LDAPUser string
	Role     Role
}

// MemberAdminRepository is the persistence port for managing book membership.
// It is separate from MembershipRepository (which only answers the read-only
// authorization questions) so the management use-case owns its own seam.
type MemberAdminRepository interface {
	// ListBookMembers returns every member of a book with their role.
	ListBookMembers(ctx context.Context, bookGUID string) ([]Member, error)
	// FindUserByEmail returns the user with the given email (Role left zero),
	// or ErrUserNotFound if no such user has been provisioned.
	FindUserByEmail(ctx context.Context, email string) (Member, error)
	// UserBookRole returns the user's role on the book and whether a membership
	// row exists at all.
	UserBookRole(ctx context.Context, userID, bookGUID string) (Role, bool, error)
	// CountBookOwners returns how many owners a book currently has.
	CountBookOwners(ctx context.Context, bookGUID string) (int, error)
	// UpsertMembership creates or updates a user's role on a book.
	UpsertMembership(ctx context.Context, userID, bookGUID string, role Role) error
	// DeleteMembership removes a user's membership on a book.
	DeleteMembership(ctx context.Context, userID, bookGUID string) error
}

// MembershipService manages who may access a book and at what role. Listing
// members needs read access; every mutation needs admin access, so an editor
// cannot grant themselves more power.
type MembershipService struct {
	repo  MemberAdminRepository
	authz *AuthzService
}

// NewMembershipService builds a MembershipService.
func NewMembershipService(repo MemberAdminRepository, authz *AuthzService) *MembershipService {
	return &MembershipService{repo: repo, authz: authz}
}

// validRole reports whether r is one of the four known roles.
func validRole(r Role) bool {
	switch r {
	case RoleViewer, RoleEditor, RoleAdmin, RoleOwner:
		return true
	default:
		return false
	}
}

// ListMembers returns the members of a book. Any member with read access may
// see who else is on the book.
func (s *MembershipService) ListMembers(ctx context.Context, userID, bookGUID string) ([]Member, error) {
	if err := s.authz.AuthorizeBook(ctx, userID, bookGUID, AccessRead); err != nil {
		return nil, err
	}
	return s.repo.ListBookMembers(ctx, bookGUID)
}

// AddMember grants a user (identified by email) a role on a book. The user must
// already exist (they have signed in at least once). Adding an existing member
// updates their role.
func (s *MembershipService) AddMember(ctx context.Context, actorUserID, bookGUID, email string, role Role) (Member, error) {
	if err := s.authz.AuthorizeBook(ctx, actorUserID, bookGUID, AccessAdmin); err != nil {
		return Member{}, err
	}
	if email == "" || !validRole(role) {
		return Member{}, ErrInvalidInput
	}
	user, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil {
		return Member{}, err
	}
	if err := s.guardLastOwner(ctx, bookGUID, user.UserID, role); err != nil {
		return Member{}, err
	}
	if err := s.repo.UpsertMembership(ctx, user.UserID, bookGUID, role); err != nil {
		return Member{}, err
	}
	user.Role = role
	return user, nil
}

// UpdateMemberRole changes an existing member's role. It refuses to demote the
// book's last owner.
func (s *MembershipService) UpdateMemberRole(ctx context.Context, actorUserID, bookGUID, targetUserID string, role Role) error {
	if err := s.authz.AuthorizeBook(ctx, actorUserID, bookGUID, AccessAdmin); err != nil {
		return err
	}
	if !validRole(role) {
		return ErrInvalidInput
	}
	if _, ok, err := s.repo.UserBookRole(ctx, targetUserID, bookGUID); err != nil {
		return err
	} else if !ok {
		return ErrUserNotFound
	}
	if err := s.guardLastOwner(ctx, bookGUID, targetUserID, role); err != nil {
		return err
	}
	return s.repo.UpsertMembership(ctx, targetUserID, bookGUID, role)
}

// RemoveMember revokes a user's access to a book. It refuses to remove the
// book's last owner.
func (s *MembershipService) RemoveMember(ctx context.Context, actorUserID, bookGUID, targetUserID string) error {
	if err := s.authz.AuthorizeBook(ctx, actorUserID, bookGUID, AccessAdmin); err != nil {
		return err
	}
	if _, ok, err := s.repo.UserBookRole(ctx, targetUserID, bookGUID); err != nil {
		return err
	} else if !ok {
		return ErrUserNotFound
	}
	// Removing a member is a demotion to "no role" for the last-owner guard.
	if err := s.guardLastOwner(ctx, bookGUID, targetUserID, ""); err != nil {
		return err
	}
	return s.repo.DeleteMembership(ctx, targetUserID, bookGUID)
}

// guardLastOwner returns ErrLastOwner if changing targetUserID to newRole would
// strip the book of its only owner. Promotions and changes that keep an owner
// are allowed.
func (s *MembershipService) guardLastOwner(ctx context.Context, bookGUID, targetUserID string, newRole Role) error {
	if newRole == RoleOwner {
		return nil
	}
	current, ok, err := s.repo.UserBookRole(ctx, targetUserID, bookGUID)
	if err != nil {
		return err
	}
	if !ok || current != RoleOwner {
		return nil // the target is not an owner, so no owner is lost
	}
	owners, err := s.repo.CountBookOwners(ctx, bookGUID)
	if err != nil {
		return err
	}
	if owners <= 1 {
		return ErrLastOwner
	}
	return nil
}
