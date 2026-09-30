package domain

type Role string

const (
	RoleCreator Role = "creator"
	RoleMember  Role = "member"
)

func VoterFor(rules Rules, role Role) bool {
	return role == RoleCreator || rules.VoterMode() == VotersAll
}
