package errs

const CodeCabalPaused Code = "cabal_paused"

func (codeFiles) Funding() map[Code]Row {
	return map[Code]Row{
		CodeCabalPaused: {Name: "CabalPaused", Kind: KindBlocked, Message: "Trading in this cabal is paused."},
	}
}
