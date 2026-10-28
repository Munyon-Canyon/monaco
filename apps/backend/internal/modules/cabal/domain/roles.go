package domain

type Role string

const (
	RoleCreator Role = "creator"
	RoleAdmin   Role = "admin"
	RoleMember  Role = "member"
)

func (r Role) IsAdmin() bool { return r == RoleCreator || r == RoleAdmin }

func VoterFor(rules Rules, role Role) bool {
	return role.IsAdmin() || rules.VoterMode() == VotersAll
}
