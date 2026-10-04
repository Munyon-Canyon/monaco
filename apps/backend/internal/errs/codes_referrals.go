package errs

const (
	CodeReferralCodeUnknown Code = "referral_code_unknown"
	CodeReferralCodePending Code = "referral_code_pending"
)

func (codeFiles) Referrals() map[Code]Row {
	return map[Code]Row{
		CodeReferralCodeUnknown: {Name: "ReferralCodeUnknown", Kind: KindNotFound, Message: "That code isn't valid"},
		CodeReferralCodePending: {
			Name: "ReferralCodePending", Kind: KindUnavailable, Retryable: true,
			Message: "Your invite code is still being created. Try again in a moment.",
		},
	}
}
