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
