package errs

const (
	CodeFundNotSent  Code = "fund_not_sent"
	CodeFundRejected Code = "fund_rejected"
	CodeFundExpired  Code = "fund_expired"

	CodeFundRefundNotLanded   Code = "fund_refund_not_landed"
	CodeFundRefundPriceable   Code = "fund_refund_priceable"
	CodeFundRefundTreasuryLow Code = "fund_refund_treasury_low"
	CodeFundRefundRejected    Code = "fund_refund_rejected"
	CodeFundRefundExpired     Code = "fund_refund_expired"
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
		CodeFundRefundNotLanded: {
			Name: "FundRefundNotLanded", Kind: KindConflict,
			Message: "Only a transfer that landed and has not settled can be refunded.",
		},
		CodeFundRefundPriceable: {
			Name: "FundRefundPriceable", Kind: KindConflict,
			Message: "The cabal can be valued again, so this transfer will settle. It was not refunded.",
		},
		CodeFundRefundTreasuryLow: {
			Name: "FundRefundTreasuryLow", Kind: KindConflict, Alert: true,
			Message: "The treasury wallet holds less than the transfer. Nothing was sent.",
		},
		CodeFundRefundRejected: {
			Name: "FundRefundRejected", Kind: KindConflict, Alert: true,
			Message: "The network rejected the refund. Nothing moved. Run the refund again.",
		},
		CodeFundRefundExpired: {
			Name: "FundRefundExpired", Kind: KindUnavailable, Retryable: true,
			Message: "The refund expired before it landed. Nothing moved. Run the refund again.",
		},
	}
}
