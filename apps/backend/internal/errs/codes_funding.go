package errs

const (
	CodeCabalPaused      Code = "cabal_paused"
	CodeCabalStillPaused Code = "cabal_still_paused"
)

func fundingRows() map[Code]Row {
	return map[Code]Row{
		CodeCabalPaused: {Name: "CabalPaused", Kind: KindBlocked, Message: "Trading in this cabal is paused."},
		CodeCabalStillPaused: {
			Name: "CabalStillPaused", Kind: KindConflict,
			Message: "The ops pause is lifted, but trading in this cabal is still paused for another reason.",
		},
	}
}
