package errs

const (
	CodeConservationBroken Code = "conservation_broken"
	CodePricesStale        Code = "prices_stale"
	CodeRankingRunsStalled Code = "ranking_runs_stalled"
)

func (codeFiles) Ranking() map[Code]Row {
	return map[Code]Row{
		CodeConservationBroken: {
			Name:    "ConservationBroken",
			Kind:    KindInternal,
			Alert:   true,
			Message: "Something went wrong.",
		},
		CodePricesStale: {
			Name: "PricesStale", Kind: KindUnavailable,
			Message: "Prices are temporarily unavailable. Try again in a moment.",
		},
		CodeRankingRunsStalled: {
			Name:    "RankingRunsStalled",
			Kind:    KindInternal,
			Alert:   true,
			Message: "Something went wrong.",
		},
	}
}
