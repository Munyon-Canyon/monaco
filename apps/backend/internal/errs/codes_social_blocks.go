package errs

const (
	CodeCannotBlockSelf      Code = "cannot_block_self"
	CodeFollowBlocked        Code = "follow_blocked"
	CodeReportTargetNotFound Code = "report_target_not_found"
)

func (codeFiles) SocialBlocks() map[Code]Row {
	return map[Code]Row{
		CodeCannotBlockSelf: {Name: "CannotBlockSelf", Kind: KindInvalid, Message: "You cannot block yourself."},
		CodeFollowBlocked:   {Name: "FollowBlocked", Kind: KindForbidden, Message: "You cannot follow this account."},
		CodeReportTargetNotFound: {
			Name: "ReportTargetNotFound", Kind: KindNotFound, Message: "That item is no longer available.",
		},
	}
}
