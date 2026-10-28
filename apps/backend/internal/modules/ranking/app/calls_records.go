package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CallRecords struct{ q *sqlc.Queries }

var _ port.Calls = CallRecords{}

func NewCallRecords(db sqlc.DBTX) CallRecords { return CallRecords{q: sqlc.New(db)} }

func (r CallRecords) Records(
	ctx context.Context, userIDs []ids.UserID, cabalID *ids.CabalID, p port.Period,
) (map[ids.UserID]port.CallRecord, error) {
	const op = "ranking.CallRecords.Records"
	out := make(map[ids.UserID]port.CallRecord, len(userIDs))
	if len(userIDs) == 0 {
		return out, nil
	}
	users := make([]uuid.UUID, len(userIDs))
	for i, id := range userIDs {
		users[i] = id.UUID()
		out[id] = port.CallRecord{}
	}
	var scope []uuid.UUID
	if cabalID != nil {
		scope = []uuid.UUID{cabalID.UUID()}
	}
	rows, err := r.q.UserCallRecords(ctx, sqlc.UserCallRecordsParams{
		UserIds: users, CabalIds: scope, WindowStart: p.Start, WindowEnd: p.End,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, row := range rows {
		out[ids.UserIDFrom(row.UserID)] = port.CallRecord{
			Right: int(row.RightCalls), Resolved: int(row.Resolved), Voted: int(row.Voted),
			Eligible: int(row.Eligible),
		}
	}
	pending, err := r.q.UserPendingCallSymbols(ctx, sqlc.UserPendingCallSymbolsParams{
		UserIds: users, CabalIds: scope, WindowEnd: p.End,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, row := range pending {
		id := ids.UserIDFrom(row.UserID)
		record := out[id]
		record.PendingSymbols = append(record.PendingSymbols, row.Symbol.String)
		out[id] = record
	}
	return out, nil
}

func (r CallRecords) CabalRecords(
	ctx context.Context, cabalIDs []ids.CabalID, p port.Period,
) (map[ids.CabalID]port.CallRecord, error) {
	out := make(map[ids.CabalID]port.CallRecord, len(cabalIDs))
	if len(cabalIDs) == 0 {
		return out, nil
	}
	cabals := make([]uuid.UUID, len(cabalIDs))
	for i, id := range cabalIDs {
		cabals[i] = id.UUID()
		out[id] = port.CallRecord{}
	}
	rows, err := r.q.CabalCallRecords(ctx, sqlc.CabalCallRecordsParams{
		CabalIds: cabals, WindowStart: p.Start, WindowEnd: p.End,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "ranking.CallRecords.CabalRecords")
	}
	for _, row := range rows {
		out[ids.CabalIDFrom(row.CabalID)] = port.CallRecord{Right: int(row.RightCalls), Resolved: int(row.Resolved)}
	}
	return out, nil
}
