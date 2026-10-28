package feed

type Outcome string

const (
	OutcomePassed Outcome = "passed"
	OutcomeFailed Outcome = "failed"
)

type Shape string

const (
	ShapeCabal  Shape = "cabal"
	ShapeMember Shape = "member"
)

type AssetKind string

const (
	AssetKindEquity AssetKind = "equity"
	AssetKindPreIPO AssetKind = "pre_ipo"
)

func ResultShape(action Action, outcome Outcome, assetKind AssetKind) Shape {
	switch {
	case outcome != OutcomePassed:
		return ShapeCabal
	case action == ActionSell:
		return ShapeMember
	case assetKind == AssetKindPreIPO:
		return ShapeMember
	default:
		return ShapeCabal
	}
}
