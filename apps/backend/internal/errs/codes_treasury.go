package errs

const (
	CodePotValueZero      Code = "pot_value_zero"
	CodeLedgerUnbalanced  Code = "ledger_unbalanced"
	CodeInsufficientFunds Code = "insufficient_funds"
)

func treasuryRows() map[Code]Row {
	return map[Code]Row{
		CodePotValueZero: {Name: "PotValueZero", Kind: KindBlocked, Message: "This cabal's pot has no value."},
		CodeLedgerUnbalanced: {
			Name: "LedgerUnbalanced", Kind: KindInternal, Alert: true, Message: "Something went wrong.",
		},
		CodeInsufficientFunds: {
			Name: "InsufficientFunds", Kind: KindBlocked, Message: "There isn't enough USDC for that.",
		},
	}
}
