package observability

var RankingSnapshotsThinned = Msg{
	Name:     "ranking.snapshots.thinned",
	Required: []string{"deleted", "before"},
}

var RankingCabalExcluded = Msg{
	Name:     "ranking.cabal.excluded",
	Required: []string{"cabal", "reason"},
}
