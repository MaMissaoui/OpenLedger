package httpapi

import (
	"context"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// authStub satisfies the cross-cutting auth floor every authed route needs:
// app.ProvisionRepository (requireAuth) and app.MembershipRepository (authz).
// The zero value provisions "user-1" and grants owner access, so most tests
// rely on the defaults authedServer wires; set a field to exercise a
// 401/403/404 path.
type authStub struct {
	userID          string   // FindOrCreateLDAPUser result (default "user-1")
	noMembership    bool     // UserBookRole reports no membership row -> 403
	role            app.Role // membership role (default owner)
	accountUnknown  bool     // BookGUIDForAccount -> ErrAccountNotFound -> 404
	accountBookGUID string   // book for BookGUIDForAccount (default "book-1")
	splitUnknown    bool     // AccountGUIDForSplit -> ErrSplitNotFound -> 404
}

func (a *authStub) FindOrCreateLDAPUser(context.Context, string, string) (string, error) {
	if a.userID != "" {
		return a.userID, nil
	}
	return "user-1", nil
}

func (a *authStub) UserBookRole(context.Context, string, string) (app.Role, bool, error) {
	if a.noMembership {
		return "", false, nil
	}
	if a.role != "" {
		return a.role, true, nil
	}
	return app.RoleOwner, true, nil
}

func (a *authStub) BookGUIDForAccount(context.Context, string) (string, error) {
	if a.accountUnknown {
		return "", app.ErrAccountNotFound
	}
	if a.accountBookGUID != "" {
		return a.accountBookGUID, nil
	}
	return "book-1", nil
}

func (a *authStub) AccountGUIDForSplit(context.Context, string) (string, error) {
	if a.splitUnknown {
		return "", app.ErrSplitNotFound
	}
	return "checking", nil
}

// authedServer builds a Server from svcs, defaulting the auth floor
// (Provision + Authz) to a granted-access authStub when the test leaves them
// nil. Tests that exercise authorization pass their own configured stub via
// Services{Provision: ..., Authz: ...}.
func authedServer(svcs Services) http.Handler {
	if svcs.Provision == nil {
		svcs.Provision = app.NewProvisionService(&authStub{})
	}
	if svcs.Authz == nil {
		svcs.Authz = app.NewAuthzService(&authStub{})
	}
	return (&Server{Services: svcs}).Routes()
}
