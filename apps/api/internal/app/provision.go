package app

import "context"

// ProvisionRepository persists LDAP-provisioned users.
type ProvisionRepository interface {
	// FindOrCreateLDAPUser returns the Postgres UUID for an LDAP-authenticated
	// user, creating an org + user row pair on the first login if needed. The
	// email is stored on creation but not updated on subsequent logins.
	FindOrCreateLDAPUser(ctx context.Context, ldapUID, email string) (userID string, err error)
}

// ProvisionService creates Postgres user records for LDAP-authenticated users
// on their first request (just-in-time provisioning). Subsequent logins are
// satisfied by the existing row; no round-trip to lldap is required.
type ProvisionService struct {
	repo ProvisionRepository
}

// NewProvisionService builds a ProvisionService backed by repo.
func NewProvisionService(repo ProvisionRepository) *ProvisionService {
	return &ProvisionService{repo: repo}
}

// ProvisionUser returns the user's Postgres UUID, creating a new org + user
// row on first call. It is idempotent: calling it multiple times with the same
// ldapUID always returns the same UUID.
func (s *ProvisionService) ProvisionUser(ctx context.Context, ldapUID, email string) (string, error) {
	return s.repo.FindOrCreateLDAPUser(ctx, ldapUID, email)
}
