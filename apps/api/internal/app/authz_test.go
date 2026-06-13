package app

import "testing"

// TestRolePermits locks the role/access policy: viewers are read-only; editor
// and above may write; unknown roles grant nothing.
func TestRolePermits(t *testing.T) {
	cases := []struct {
		role        Role
		read, write bool
	}{
		{RoleViewer, true, false},
		{RoleEditor, true, true},
		{RoleAdmin, true, true},
		{RoleOwner, true, true},
		{Role("bogus"), false, false},
		{Role(""), false, false},
	}
	for _, c := range cases {
		if got := c.role.permits(AccessRead); got != c.read {
			t.Errorf("%q permits read = %v, want %v", c.role, got, c.read)
		}
		if got := c.role.permits(AccessWrite); got != c.write {
			t.Errorf("%q permits write = %v, want %v", c.role, got, c.write)
		}
	}
}
