package errs

const (
	CodeDust         Code = "dust"
	CodeUnknownAsset Code = "unknown_asset"
)

func (codeFiles) FundingWatch() map[Code]Row {
	return map[Code]Row{
		CodeDust: {
			Name: "Dust", Kind: KindInvalid,
			Message: "A transfer into a cabal treasury was worth under $1, so it was recorded and ignored.",
		},
		CodeUnknownAsset: {
			Name:    "UnknownAsset",
			Kind:    KindInvalid,
			Message: "A transfer into a cabal treasury was a token Monaco doesn't list, so it was recorded and ignored.",
		},
	}
}
