package errs

const CodeWithdrawToOwnWallet Code = "withdraw_to_own_wallet"

func (codeFiles) FundingWithdrawals() map[Code]Row {
	return map[Code]Row{
		CodeWithdrawToOwnWallet: {
			Name: "WithdrawToOwnWallet", Kind: KindInvalid,
			Message: "That address is your Monaco wallet. Paste an address outside Monaco.",
		},
	}
}
