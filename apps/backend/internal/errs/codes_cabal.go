package errs

const (
	CodeCabalNotFound              Code = "cabal_not_found"
	CodeNotCabalMember             Code = "not_cabal_member"
	CodeNotCabalCreator            Code = "not_cabal_creator"
	CodeCannotRevokeAccess         Code = "cannot_revoke_access"
	CodeCabalBanned                Code = "cabal_banned"
	CodeAlreadyMember              Code = "already_member"
	CodeJoinNeedsRequest           Code = "join_needs_request"
	CodeRequestNotNeeded           Code = "request_not_needed"
	CodeRequestPending             Code = "request_pending"
	CodeAccessRequestNotPending    Code = "access_request_not_pending"
	CodeInviteExpired              Code = "invite_expired"
	CodeLeaveHoldsShares           Code = "leave_holds_shares"
	CodeLeaveLastMemberPotNotEmpty Code = "leave_last_member_pot_not_empty"
	CodeLeaveCreatorWithMembers    Code = "leave_creator_with_members"
)

func cabalRows() map[Code]Row {
	return map[Code]Row{
		CodeCabalNotFound: {Name: "CabalNotFound", Kind: KindNotFound, Message: "We could not find that cabal."},
		CodeNotCabalMember: {
			Name: "NotCabalMember", Kind: KindForbidden, Message: "You are not a member of this cabal.",
		},
		CodeNotCabalCreator: {
			Name: "NotCabalCreator", Kind: KindForbidden, Message: "Only the creator of this cabal can do that.",
		},
		CodeCannotRevokeAccess: {
			Name: "CannotRevokeAccess", Kind: KindForbidden, Message: "You cannot cancel this request or invite.",
		},
		CodeCabalBanned: {
			Name: "CabalBanned", Kind: KindBlocked,
			Message: "This cabal is banned. Members can still cash out and leave.",
		},
		CodeAlreadyMember: {
			Name: "AlreadyMember", Kind: KindBlocked, Message: "That person is already in this cabal.",
		},
		CodeJoinNeedsRequest: {
			Name: "JoinNeedsRequest", Kind: KindBlocked,
			Message: "This cabal needs the creator's approval. Send a request to join.",
		},
		CodeRequestNotNeeded: {
			Name: "RequestNotNeeded", Kind: KindBlocked, Message: "This cabal is open. Join it directly.",
		},
		CodeRequestPending: {
			Name: "RequestPending", Kind: KindBlocked,
			Message: "A request or invite for this cabal is already pending.",
		},
		CodeAccessRequestNotPending: {
			Name: "AccessRequestNotPending", Kind: KindBlocked,
			Message: "This request or invite is no longer pending.",
		},
		CodeInviteExpired: {Name: "InviteExpired", Kind: KindBlocked, Message: "This invite has expired."},
		CodeLeaveHoldsShares: {
			Name: "LeaveHoldsShares", Kind: KindBlocked,
			Message: "Cash out your share of the pot before you leave this cabal.",
		},
		CodeLeaveLastMemberPotNotEmpty: {
			Name: "LeaveLastMemberPotNotEmpty", Kind: KindBlocked,
			Message: "The pot still holds money, so the last member cannot leave yet.",
		},
		CodeLeaveCreatorWithMembers: {
			Name: "LeaveCreatorWithMembers", Kind: KindBlocked,
			Message: "The creator cannot leave while other members remain.",
		},
	}
}
