package errs

const (
	CodeWithdrawToOwnWallet   Code = "withdraw_to_own_wallet"
	CodeWithdrawNoUSDCAccount Code = "withdraw_no_usdc_account"
	CodeStepUpRequired        Code = "step_up_required"
	CodeStepUpMismatch        Code = "step_up_mismatch"
	CodeReauthRequired        Code = "reauth_required"
)

func (codeFiles) FundingWithdrawals() map[Code]Row {
	return map[Code]Row{
		CodeWithdrawToOwnWallet: {
			Name: "WithdrawToOwnWallet", Kind: KindInvalid,
			Message: "That address is your Monaco wallet. Paste an address outside Monaco.",
		},
		CodeWithdrawNoUSDCAccount: {
			Name:    "WithdrawNoUSDCAccount",
			Kind:    KindInvalid,
			Message: "This address can't receive USDC yet. Ask the owner to open a USDC account, or use another address.",
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
