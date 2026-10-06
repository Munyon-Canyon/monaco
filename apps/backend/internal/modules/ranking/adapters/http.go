package adapters

import (
	"context"
	"log/slog"
	"math"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
)

type HTTP struct {
	Boards app.Boards
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetCabalsLeaderboard(
	ctx context.Context, req api.GetCabalsLeaderboardRequestObject,
) (api.GetCabalsLeaderboardResponseObject, error) {
	var rng *string
	if req.Params.Range != nil {
		raw := string(*req.Params.Range)
		rng = &raw
	}
	in, err := readOf(domain.BoardCabals, rng, req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	page, err := in.Run(ctx, h.Boards)
	if err != nil {
		return nil, err
	}
	out, err := wirePage(page, in, api.Cabal)
	if err != nil {
		return nil, err
	}
	return api.GetCabalsLeaderboard200JSONResponse(out), nil
}

func readOf(board domain.Board, rng, cursor *string, limit *int) (app.ReadBoard, error) {
	in := app.ReadBoard{Board: string(board), Range: domain.RangeAll}
	var err error
	if rng != nil {
		if in.Range, err = domain.ParseRange(*rng); err != nil {
			return app.ReadBoard{}, err
		}
	}
	if cursor != nil {
		if in.Cursor, err = domain.DecodeCursor(*cursor); err != nil {
			return app.ReadBoard{}, err
		}
	}
	if in.Limit, err = domain.ParseLimit(limit); err != nil {
		return app.ReadBoard{}, err
	}
	return in, nil
}

func wirePage(page domain.BoardPage, in app.ReadBoard, kind api.LeaderboardSubjectKind) (api.LeaderboardPage, error) {
	rows := make([]api.LeaderboardRow, 0, len(page.Rows))
	for _, entry := range page.Rows {
		row, err := wireRow(entry, kind)
		if err != nil {
			return api.LeaderboardPage{}, err
		}
		rows = append(rows, row)
	}
	return api.LeaderboardPage{
		RunId:      page.RunID,
		Board:      in.Board,
		Range:      api.LeaderboardPageRange(in.Range),
		ComputedAt: page.ComputedAt,
		PricesAsOf: page.PricesAsOf,
		Rows:       rows,
		NextCursor: page.NextCursor,
	}, nil
}

func wireRow(entry domain.Entry, kind api.LeaderboardSubjectKind) (api.LeaderboardRow, error) {
	value := entry.Value.Uint64()
	if value > math.MaxInt64 || entry.Rank < 0 || entry.Rank > math.MaxInt32 {
		return api.LeaderboardRow{}, errs.New(
			errs.CodeInternal, "ranking.wireRow", slog.Uint64("value", value), slog.Int("rank", entry.Rank))
	}
	flags := make([]api.LeaderboardRowFlags, 0, len(entry.Flags))
	for _, f := range entry.Flags {
		flags = append(flags, api.LeaderboardRowFlags(f))
	}
	row := api.LeaderboardRow{
		Rank: int32(entry.Rank),
		Subject: api.LeaderboardSubject{
			Id: entry.Subject.ID, Kind: kind, Name: entry.Subject.Name,
			Handle: entry.Subject.Handle, PictureUrl: entry.Subject.PictureURL,
		},
		ValueMicros: int64(value),
		PnlMicros:   entry.PnL.Int64(),
		Flags:       flags,
	}
	if entry.Return != nil {
		bps := int64(*entry.Return)
		row.ReturnBps = &bps
	}
	return row, nil
}
