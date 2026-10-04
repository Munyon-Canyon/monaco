package domain

import "testing"

func TestRoleAllows(t *testing.T) {
	t.Parallel()
	tests := []struct {
		role     Role
		required Role
		want     bool
	}{
		{RoleViewer, RoleViewer, true},
		{RoleViewer, RoleModerator, false},
		{RoleViewer, RoleOperator, false},
		{RoleModerator, RoleViewer, true},
		{RoleModerator, RoleModerator, true},
		{RoleModerator, RoleOperator, false},
		{RoleOperator, RoleViewer, true},
		{RoleOperator, RoleModerator, true},
		{RoleOperator, RoleOperator, true},
		{"", RoleViewer, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.role)+"_"+string(tt.required), func(t *testing.T) {
			t.Parallel()
			if got := tt.role.Allows(tt.required); got != tt.want {
				t.Errorf("Allows(%q) = %t, want %t", tt.required, got, tt.want)
			}
		})
	}
}
