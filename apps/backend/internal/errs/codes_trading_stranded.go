package errs

const CodeStrandedCashTaken Code = "stranded_cash_taken"

func (codeFiles) TradingStranded() map[Code]Row {
	return map[Code]Row{
		CodeStrandedCashTaken: {
			Name: "StrandedCashTaken", Kind: KindConflict, Retryable: true,
			Message: "This trade's cash is already on its way back. Try again in a moment.",
		},
	}
}
