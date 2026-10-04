package errs

const (
	CodeAdminForbidden Code = "admin_forbidden"
	CodeReasonRequired Code = "reason_required"
)

func (codeFiles) Admin() map[Code]Row {
	return map[Code]Row{
		CodeAdminForbidden: {Name: "AdminForbidden", Kind: KindForbidden, Message: "Admin access is required."},
		CodeReasonRequired: {Name: "ReasonRequired", Kind: KindInvalid, Message: "A reason is required."},
	}
}
