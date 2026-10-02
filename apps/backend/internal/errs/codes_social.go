package errs

const (
	CodeCannotFollowSelf Code = "cannot_follow_self"
	CodeUserBanned       Code = "user_banned"
)

func socialRows() map[Code]Row {
	return map[Code]Row{
		CodeCannotFollowSelf: {Name: "CannotFollowSelf", Kind: KindInvalid, Message: "You cannot follow yourself."},
		CodeUserBanned:       {Name: "UserBanned", Kind: KindForbidden, Message: "You cannot follow this account."},
	}
}
