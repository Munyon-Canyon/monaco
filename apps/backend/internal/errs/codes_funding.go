package errs

const CodeCabalPaused Code = "cabal_paused"

func fundingRows() map[Code]Row {
	return map[Code]Row{
		CodeCabalPaused: {Name: "CabalPaused", Kind: KindBlocked, Message: "Trading in this cabal is paused."},
	}
}
