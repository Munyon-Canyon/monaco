package errs

const CodeFundNotSent Code = "fund_not_sent"

func (codeFiles) TreasuryFund() map[Code]Row {
	return map[Code]Row{
		CodeFundNotSent: {
			Name: "FundNotSent", Kind: KindUnavailable, Retryable: true,
			Message: "The transfer to the cabal was never sent. Your balance is unchanged. Try again.",
		},
	}
}
