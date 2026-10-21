package errs

const (
	CodeWithdrawToOwnWallet Code = "withdraw_to_own_wallet"
	CodeStepUpRequired      Code = "step_up_required"
	CodeStepUpMismatch      Code = "step_up_mismatch"
	CodeReauthRequired      Code = "reauth_required"
)

func (codeFiles) FundingWithdrawals() map[Code]Row {
	return map[Code]Row{
		CodeWithdrawToOwnWallet: {
			Name: "WithdrawToOwnWallet", Kind: KindInvalid,
			Message: "That address is your Monaco wallet. Paste an address outside Monaco.",
		},
		CodeStepUpRequired: {
			Name: "StepUpRequired", Kind: KindBlocked,
			Message: "Confirm this withdrawal to continue.",
		},
		CodeStepUpMismatch: {
			Name: "StepUpMismatch", Kind: KindBlocked,
			Message: "The amount or address changed after you confirmed. Confirm the withdrawal again.",
		},
		CodeReauthRequired: {
			Name: "ReauthRequired", Kind: KindBlocked,
			Message: "Sign in again to confirm this withdrawal.",
		},
	}
}
