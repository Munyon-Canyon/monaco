package domain

import (
	"cmp"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Candidate struct {
	SubjectID string
	CreatedAt time.Time
	Value     money.Micros
	PnL       money.SignedMicros
	Return    *Bps
	Flags     []Flag
}

type Row struct {
	Rank int
	Candidate
}

func Rank(cands []Candidate, keepUnranked bool) []Row {
	ranked := make([]Candidate, 0, len(cands))
	var unranked []Candidate
	for _, c := range cands {
		if c.Return != nil {
			ranked = append(ranked, c)
		} else if keepUnranked {
			unranked = append(unranked, c)
		}
	}
	slices.SortFunc(ranked, func(a, b Candidate) int {
		return cmp.Or(cmp.Compare(*b.Return, *a.Return), byAgeThenID(a, b))
	})
	slices.SortFunc(unranked, byAgeThenID)
	rows := make([]Row, 0, len(ranked)+len(unranked))
	for _, c := range slices.Concat(ranked, unranked) {
		rows = append(rows, Row{Rank: len(rows) + 1, Candidate: c})
	}
	return rows
}

func byAgeThenID(a, b Candidate) int {
	return cmp.Or(a.CreatedAt.Compare(b.CreatedAt), cmp.Compare(a.SubjectID, b.SubjectID))
}
