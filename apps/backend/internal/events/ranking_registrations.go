package events

func rankingRegistrations() []Registration {
	return []Registration{Register[RankingSnapshotWritten](TypeRankingSnapshotWritten, 1)}
}
