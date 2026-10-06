package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type previousEntriesQuery interface {
	PreviousEntriesForCabals(context.Context, sqlc.PreviousEntriesForCabalsParams) ([]sqlc.LeaderboardEntry, error)
}

func MembersBoard(cabalID uuid.UUID) string { return "cabal_members:" + cabalID.String() }
func PreviousEntries(ctx context.Context, q previousEntriesQuery, cabals []ids.CabalID) ([]Entry, error) {
	if len(cabals) == 0 {
		return []Entry{}, nil
	}
	params := sqlc.PreviousEntriesForCabalsParams{
		CabalIds:     make([]uuid.UUID, len(cabals)),
		MemberBoards: make([]string, len(cabals)),
	}
	for i, cabal := range cabals {
		params.CabalIds[i] = cabal.UUID()
		params.MemberBoards[i] = MembersBoard(cabal.UUID())
	}
	rows, err := q.PreviousEntriesForCabals(ctx, params)
	if err != nil {
		return nil, err
	}
	entries := make([]Entry, len(rows))
	for i, row := range rows {
		entries[i] = entryOf(row)
	}
	return entries, nil
}

func entryOf(row sqlc.LeaderboardEntry) Entry {
	entry := Entry{
		Board: row.Board, Range: row.Range, Rank: int(row.Rank), SubjectID: row.SubjectID,
		SubjectName: row.SubjectName, SubjectCreatedAt: row.SubjectCreatedAt,
		ValueMicros: row.ValueMicros, PnLMicros: row.PnlMicros,
		PricesAsOf: row.PricesAsOf, ComputedAt: row.ComputedAt, Flags: row.Flags,
	}
	if row.SubjectHandle.Valid {
		entry.SubjectHandle = &row.SubjectHandle.String
	}
	if row.SubjectPictureUrl.Valid {
		entry.SubjectPictureURL = &row.SubjectPictureUrl.String
	}
	if row.ReturnBps.Valid {
		entry.ReturnBps = &row.ReturnBps.Int64
	}
	return entry
}
