package errs

const (
	CodeCabalPaused  Code = "cabal_paused"
	CodeNotADeposit  Code = "not_a_deposit"
	CodeMonacoSigned Code = "monaco_signed"
	CodeUnresolved   Code = "unresolved"
)

func (codeFiles) Funding() map[Code]Row {
	return map[Code]Row{
		CodeCabalPaused:  {Name: "CabalPaused", Kind: KindBlocked, Message: "Trading in this cabal is paused."},
		CodeNotADeposit:  {Name: "NotADeposit", Kind: KindBlocked, Message: "This transfer is not a deposit."},
		CodeMonacoSigned: {Name: "MonacoSigned", Kind: KindBlocked, Message: "This transfer belongs to Monaco."},
		CodeUnresolved: {
			Name: "Unresolved", Kind: KindUnavailable, Retryable: true, Message: "This deposit is still being checked.",
		},
	}
}
