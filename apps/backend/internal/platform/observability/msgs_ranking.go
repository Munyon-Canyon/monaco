package observability

var RankingSnapshotsThinned = Msg{
	Name:     "ranking.snapshots.thinned",
	Required: []string{"deleted", "before"},
}

var RankingRangeStartSkipped = Msg{
	Name:     "ranking.range_start.skipped",
	Required: []string{"cabal", "range", "reason"},
}

var RankingCabalExcluded = Msg{
	Name:     "ranking.cabal.excluded",
	Required: []string{"cabal", "reason"},
}

var RankingCabalUnpricedListing = Msg{
	Name:     "ranking.cabal.unpriced_listing",
	Required: []string{"cabal", "listing", "reason"},
}

var RankingRunCompleted = Msg{
	Name:     "ranking.run.completed",
	Required: []string{"run_id", "rows", "excluded"},
}
