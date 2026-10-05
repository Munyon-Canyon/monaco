package observability

var RankingSnapshotsThinned = Msg{
	Name:     "ranking.snapshots.thinned",
	Required: []string{"deleted", "before"},
}
