package errs

const (
	CodeFundNotSent  Code = "fund_not_sent"
	CodeFundRejected Code = "fund_rejected"
	CodeFundExpired  Code = "fund_expired"
)

func (codeFiles) TreasuryFund() map[Code]Row {
	return map[Code]Row{
		CodeFundNotSent: {
			Name: "FundNotSent", Kind: KindUnavailable, Retryable: true,
			Message: "The transfer to the cabal was never sent. Your balance is unchanged. Try again.",
		},
		CodeFundRejected: {
			Name: "FundRejected", Kind: KindConflict,
			Message: "The network rejected the transfer to the cabal. Your balance is unchanged.",
		},
		CodeFundExpired: {
			Name: "FundExpired", Kind: KindUnavailable, Retryable: true,
			Message: "The transfer to the cabal expired before it landed. Your balance is unchanged. Try again.",
		},
	}
}
