package errs

const (
	CodeSameApprover           Code = "same_approver"
	CodeCabalNotActive         Code = "cabal_not_active"
	CodeApprovalAlreadyPending Code = "approval_already_pending"
	CodeApprovalNotPending     Code = "approval_not_pending"
	CodeApprovalExpired        Code = "approval_expired"
)

func (codeFiles) AdminApprovals() map[Code]Row {
	return map[Code]Row{
		CodeSameApprover: {
			Name: "SameApprover", Kind: KindForbidden,
			Message: "A different operator must approve this request.",
		},
		CodeCabalNotActive: {
			Name: "CabalNotActive", Kind: KindConflict, Message: "This cabal is not active, so it cannot be banned.",
		},
		CodeApprovalAlreadyPending: {
			Name: "ApprovalAlreadyPending", Kind: KindConflict,
			Message: "A request for this action is already waiting for approval.",
		},
		CodeApprovalNotPending: {
			Name: "ApprovalNotPending", Kind: KindConflict, Message: "This request was already decided.",
		},
		CodeApprovalExpired: {
			Name: "ApprovalExpired", Kind: KindConflict, Message: "This request expired. Ask for a new one.",
		},
	}
}
