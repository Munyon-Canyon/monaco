package errs

const (
	CodePotValueZero       Code = "pot_value_zero"
	CodePriceUnavailable   Code = "price_unavailable"
	CodeLedgerUnbalanced   Code = "ledger_unbalanced"
	CodeInsufficientFunds  Code = "insufficient_funds"
	CodeInsufficientShares Code = "insufficient_shares"
	CodeCashOutInProgress  Code = "cash_out_in_progress"
	CodeSaleShort          Code = "sale_short"
	CodePayoutFailed       Code = "payout_failed"
	CodePotValueChanged    Code = "pot_value_changed"
)

func (codeFiles) Treasury() map[Code]Row {
	return map[Code]Row{
		CodePotValueZero: {Name: "PotValueZero", Kind: KindBlocked, Message: "This cabal's pot has no value."},
		CodePriceUnavailable: {
			Name: "PriceUnavailable", Kind: KindUnavailable, Retryable: true,
			Message: "Prices are temporarily unavailable. Try again in a moment.",
		},
		CodeLedgerUnbalanced: {
			Name: "LedgerUnbalanced", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
		CodeInsufficientFunds: {
			Name: "InsufficientFunds", Kind: KindBlocked, Message: "There isn't enough USDC for that.",
		},
		CodeInsufficientShares: {
			Name: "InsufficientShares", Kind: KindBlocked, Message: "You don't have enough shares for that.",
		},
		CodeCashOutInProgress: {
			Name: "CashOutInProgress", Kind: KindConflict, Message: "A cash out is already in progress.",
		},
		CodePayoutFailed: {
			Name: "PayoutFailed", Kind: KindInternal, Alert: true,
			Message: "The cash out didn't go through. Your shares are back in the cabal.",
		},
		CodePotValueChanged: {
			Name: "PotValueChanged", Kind: KindConflict, Retryable: true,
			Message: "The pot's value changed. Check the amount and try again.",
		},
		CodeSaleShort: {
			Name: "SaleShort", Kind: KindBlocked, Message: "The sale raised less than this cash out's slice.",
		},
	}
}
