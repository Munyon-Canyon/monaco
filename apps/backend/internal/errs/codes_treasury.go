package errs

const (
	CodePotValueZero      Code = "pot_value_zero"
	CodePriceUnavailable  Code = "price_unavailable"
	CodeLedgerUnbalanced  Code = "ledger_unbalanced"
	CodeInsufficientFunds Code = "insufficient_funds"
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
	}
}
