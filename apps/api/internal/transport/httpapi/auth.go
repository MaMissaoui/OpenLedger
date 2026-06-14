package httpapi

import (
	"context"
	"errors"
	"net/http"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// ctxKey is the unexported type for request-context keys, avoiding collisions
// with keys set by other packages.
type ctxKey int

const identityKey ctxKey = iota

// identity holds the Authelia-verified user for the duration of a request.
type identity struct {
	UserID   string // UUID from the users table (JIT-provisioned on first login)
	LDAPUser string // uid attribute from lldap, e.g. "alice"
	Email    string // mail attribute from lldap
}

// requireAuth wraps a handler so it only runs when Authelia (via Traefik's
// forwardAuth middleware) has authenticated the request. Authelia sets
// Remote-User and Remote-Email on every proxied request that passes
// verification; a missing Remote-User header means the request bypassed the
// proxy and must be rejected.
//
// On the first request from a user, ProvisionService creates a users row so
// the membership / authz system has a stable UUID to reference.
func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ldapUID := r.Header.Get("Remote-User")
		email := r.Header.Get("Remote-Email")
		if ldapUID == "" {
			writeError(w, http.StatusUnauthorized, "not authenticated")
			return
		}
		userID, err := s.provision.ProvisionUser(r.Context(), ldapUID, email)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "could not provision user")
			return
		}
		ctx := context.WithValue(r.Context(), identityKey, identity{
			UserID:   userID,
			LDAPUser: ldapUID,
			Email:    email,
		})
		next(w, r.WithContext(ctx))
	}
}

// actorFromContext returns the AuditActor for the authenticated request. It is
// only meaningful on handlers wrapped by requireAuth.
func actorFromContext(ctx context.Context) app.AuditActor {
	id, ok := ctx.Value(identityKey).(identity)
	if !ok {
		return app.AuditActor{}
	}
	return app.AuditActor{UserID: id.UserID}
}

// authorizeBook enforces that the context user's role on bookGUID permits the
// need-level action. It writes the error response and returns false when the
// request must stop.
func (s *Server) authorizeBook(w http.ResponseWriter, r *http.Request, bookGUID string, need app.Access) bool {
	actor := actorFromContext(r.Context())
	return !writeAuthzError(w, s.authz.AuthorizeBook(r.Context(), actor.UserID, bookGUID, need))
}

// authorizeAccount enforces the user's role on the book the account belongs to.
func (s *Server) authorizeAccount(w http.ResponseWriter, r *http.Request, accountGUID string, need app.Access) bool {
	actor := actorFromContext(r.Context())
	return !writeAuthzError(w, s.authz.AuthorizeAccount(r.Context(), actor.UserID, accountGUID, need))
}

// authorizeAccounts enforces the user's role on the book(s) the given accounts belong to.
func (s *Server) authorizeAccounts(w http.ResponseWriter, r *http.Request, accountGUIDs []string, need app.Access) bool {
	actor := actorFromContext(r.Context())
	return !writeAuthzError(w, s.authz.AuthorizeAccounts(r.Context(), actor.UserID, accountGUIDs, need))
}

// authorizeSplit enforces the user's role on the book the split's account
// belongs to.
func (s *Server) authorizeSplit(w http.ResponseWriter, r *http.Request, splitGUID string, need app.Access) bool {
	actor := actorFromContext(r.Context())
	return !writeAuthzError(w, s.authz.AuthorizeSplit(r.Context(), actor.UserID, splitGUID, need))
}

// writeAuthzError maps an authorization error to an HTTP response, returning
// whether it handled one. A nil error is a no-op returning false.
func writeAuthzError(w http.ResponseWriter, err error) bool {
	switch {
	case err == nil:
		return false
	case errors.Is(err, app.ErrForbidden):
		writeError(w, http.StatusForbidden, "you do not have access to this book")
	case errors.Is(err, app.ErrInsufficientRole):
		writeError(w, http.StatusForbidden, "your role does not permit this action")
	case errors.Is(err, app.ErrBookNotFound):
		writeError(w, http.StatusNotFound, "book not found")
	case errors.Is(err, app.ErrAccountNotFound):
		writeError(w, http.StatusNotFound, "account not found")
	case errors.Is(err, app.ErrSplitNotFound):
		writeError(w, http.StatusNotFound, "split not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal error")
	}
	return true
}
