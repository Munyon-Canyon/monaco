package errs

const (
	CodeReferralCodeUnknown     Code = "referral_code_unknown"
	CodeReferralCodePending     Code = "referral_code_pending"
	CodeReferralSelf            Code = "referral_self"
	CodeReferralAlreadyAttached Code = "referral_already_attached"
	CodeReferralWindowClosed    Code = "referral_window_closed"
)

func (codeFiles) Referrals() map[Code]Row {
	return map[Code]Row{
		CodeReferralCodeUnknown: {Name: "ReferralCodeUnknown", Kind: KindNotFound, Message: "That code isn't valid"},
		CodeReferralCodePending: {
			Name: "ReferralCodePending", Kind: KindUnavailable, Retryable: true,
			Message: "Your invite code is still being created. Try again in a moment.",
		},
		CodeReferralSelf: {
			Name: "ReferralSelf", Kind: KindBlocked, Message: "You can't use your own invite.",
		},
		CodeReferralAlreadyAttached: {
			Name: "ReferralAlreadyAttached", Kind: KindBlocked, Message: "You've already used an invite.",
		},
		CodeReferralWindowClosed: {
			Name: "ReferralWindowClosed", Kind: KindBlocked, Message: "Invites work only for new accounts.",
		},
	}
}
