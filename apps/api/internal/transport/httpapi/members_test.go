package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/openledger/openledger/apps/api/internal/app"
)

// memberFake satisfies app.MemberAdminRepository for member-management tests.
type memberFake struct {
	members map[string]app.Member // by userID
	byEmail map[string]app.Member // by lower(email)
}

func newMemberFake() *memberFake {
	return &memberFake{members: map[string]app.Member{}, byEmail: map[string]app.Member{}}
}

// seedUser registers a provisioned user (findable by email) and, if role is
// non-empty, a membership on the book.
func (f *memberFake) seedUser(userID, email string, role app.Role) {
	m := app.Member{UserID: userID, Email: email, LDAPUser: strings.Split(email, "@")[0]}
	f.byEmail[strings.ToLower(email)] = m
	if role != "" {
		m.Role = role
		f.members[userID] = m
	}
}

func (f *memberFake) ListBookMembers(context.Context, string) ([]app.Member, error) {
	out := make([]app.Member, 0, len(f.members))
	for _, m := range f.members {
		out = append(out, m)
	}
	return out, nil
}

func (f *memberFake) FindUserByEmail(_ context.Context, email string) (app.Member, error) {
	if m, ok := f.byEmail[strings.ToLower(email)]; ok {
		return m, nil
	}
	return app.Member{}, app.ErrUserNotFound
}

func (f *memberFake) UserBookRole(_ context.Context, userID, _ string) (app.Role, bool, error) {
	if m, ok := f.members[userID]; ok {
		return m.Role, true, nil
	}
	return "", false, nil
}

func (f *memberFake) CountBookOwners(context.Context, string) (int, error) {
	n := 0
	for _, m := range f.members {
		if m.Role == app.RoleOwner {
			n++
		}
	}
	return n, nil
}

func (f *memberFake) UpsertMembership(_ context.Context, userID, _ string, role app.Role) error {
	m := f.members[userID]
	if m.UserID == "" {
		m = f.byEmail[strings.ToLower(emailFor(f, userID))]
		m.UserID = userID
	}
	m.Role = role
	f.members[userID] = m
	return nil
}

func (f *memberFake) DeleteMembership(_ context.Context, userID, _ string) error {
	delete(f.members, userID)
	return nil
}

func emailFor(f *memberFake, userID string) string {
	for _, m := range f.byEmail {
		if m.UserID == userID {
			return m.Email
		}
	}
	return ""
}

// memberServer wires a MembershipService over the fake. A nil authz grants
// owner access (the authStub default); pass one to exercise role gating.
func memberServer(f *memberFake, authz *app.AuthzService) http.Handler {
	if authz == nil {
		authz = app.NewAuthzService(&authStub{})
	}
	return authedServer(Services{Membership: app.NewMembershipService(f, authz), Authz: authz})
}

func memberReq(h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, withAuth(r))
	return rec
}

func TestListMembers200(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	f.seedUser("u2", "view@x.com", app.RoleViewer)
	rec := memberReq(memberServer(f, nil), "GET", "/api/v1/books/book-1/members", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Members []map[string]any
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &resp)
	if len(resp.Members) != 2 {
		t.Errorf("got %d members, want 2", len(resp.Members))
	}
}

func TestAddMemberByEmail201(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	f.seedUser("u2", "new@x.com", "") // provisioned but not yet a member
	rec := memberReq(memberServer(f, nil), "POST", "/api/v1/books/book-1/members",
		`{"email":"new@x.com","role":"editor"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", rec.Code, rec.Body.String())
	}
	if f.members["u2"].Role != app.RoleEditor {
		t.Errorf("u2 role = %q, want editor", f.members["u2"].Role)
	}
}

func TestAddMemberUnknownEmail404(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	rec := memberReq(memberServer(f, nil), "POST", "/api/v1/books/book-1/members",
		`{"email":"ghost@x.com","role":"editor"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404; body = %s", rec.Code, rec.Body.String())
	}
}

func TestAddMemberInvalidRole400(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u2", "new@x.com", "")
	rec := memberReq(memberServer(f, nil), "POST", "/api/v1/books/book-1/members",
		`{"email":"new@x.com","role":"superuser"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400; body = %s", rec.Code, rec.Body.String())
	}
}

func TestUpdateMemberRole204(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	f.seedUser("u2", "view@x.com", app.RoleViewer)
	rec := memberReq(memberServer(f, nil), "PATCH", "/api/v1/books/book-1/members/u2",
		`{"role":"editor"}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rec.Code, rec.Body.String())
	}
	if f.members["u2"].Role != app.RoleEditor {
		t.Errorf("u2 role = %q, want editor", f.members["u2"].Role)
	}
}

func TestRemoveMember204(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	f.seedUser("u2", "view@x.com", app.RoleViewer)
	rec := memberReq(memberServer(f, nil), "DELETE", "/api/v1/books/book-1/members/u2", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := f.members["u2"]; ok {
		t.Error("u2 should have been removed")
	}
}

func TestCannotRemoveLastOwner409(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner) // the only owner
	rec := memberReq(memberServer(f, nil), "DELETE", "/api/v1/books/book-1/members/u1", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
	if _, ok := f.members["u1"]; !ok {
		t.Error("the last owner must not be removed")
	}
}

func TestCannotDemoteLastOwner409(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u1", "owner@x.com", app.RoleOwner)
	rec := memberReq(memberServer(f, nil), "PATCH", "/api/v1/books/book-1/members/u1",
		`{"role":"editor"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}
}

func TestManageMembersForbiddenForNonAdmin(t *testing.T) {
	f := newMemberFake()
	f.seedUser("u2", "new@x.com", "")
	// An editor (below admin) may not manage members.
	authz := app.NewAuthzService(&authStub{role: app.RoleEditor})
	rec := memberReq(memberServer(f, authz), "POST", "/api/v1/books/book-1/members",
		`{"email":"new@x.com","role":"viewer"}`)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403; body = %s", rec.Code, rec.Body.String())
	}
}
