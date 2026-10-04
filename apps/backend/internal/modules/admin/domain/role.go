package domain

type Role string

const (
	RoleViewer    Role = "viewer"
	RoleModerator Role = "moderator"
	RoleOperator  Role = "operator"
)

func (r Role) Allows(required Role) bool {
	return r.rank() >= required.rank()
}

func (r Role) rank() int {
	switch r {
	case RoleOperator:
		return 3
	case RoleModerator:
		return 2
	case RoleViewer:
		return 1
	default:
		return 0
	}
}
