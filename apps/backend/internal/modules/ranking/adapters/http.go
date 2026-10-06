package adapters

import (
	"context"
	"log/slog"
	"math"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type HTTP struct {
	Boards app.Boards
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetCabalsLeaderboard(
	ctx context.Context, req api.GetCabalsLeaderboardRequestObject,
) (api.GetCabalsLeaderboardResponseObject, error) {
	page, err := h.serve(ctx, string(domain.BoardCabals), api.Cabal, nil,
		textOf(req.Params.Range), req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetCabalsLeaderboard200JSONResponse(page), nil
}

func (h HTTP) GetPeopleLeaderboard(
	ctx context.Context, req api.GetPeopleLeaderboardRequestObject,
) (api.GetPeopleLeaderboardResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	page, err := h.serve(ctx, string(domain.BoardPeople), api.User, &viewer,
		textOf(req.Params.Range), req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetPeopleLeaderboard200JSONResponse(page), nil
}

func (h HTTP) serve(
	ctx context.Context, board string, kind api.LeaderboardSubjectKind, viewer *ids.UserID,
	rng, cursor *string, limit *int,
) (api.LeaderboardPage, error) {
	in, err := readOf(board, rng, cursor, limit)
	if err != nil {
		return api.LeaderboardPage{}, err
	}
	if viewer != nil {
		id := viewer.UUID()
		in.Viewer = &id
	}
	page, err := in.Run(ctx, h.Boards)
	if err != nil {
		return api.LeaderboardPage{}, err
	}
	return wirePage(page, in, kind)
}

func textOf[T ~string](v *T) *string {
	if v == nil {
		return nil
	}
	raw := string(*v)
	return &raw
}

func readOf(board string, rng, cursor *string, limit *int) (app.ReadBoard, error) {
	in := app.ReadBoard{Board: board, Range: domain.RangeAll}
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
	out := api.LeaderboardPage{
		RunId:      page.RunID,
		Board:      in.Board,
		Range:      api.LeaderboardPageRange(in.Range),
		ComputedAt: page.ComputedAt,
		PricesAsOf: page.PricesAsOf,
		Rows:       rows,
		NextCursor: page.NextCursor,
	}
	if page.Me != nil && page.Me.Return != nil {
		me, err := wireRow(*page.Me, kind)
		if err != nil {
			return api.LeaderboardPage{}, err
		}
		out.Me = &me
	}
	return out, nil
}

func caller(ctx context.Context) (ids.UserID, error) {
	const op = "ranking.caller"
	actor, ok := auth.ActorFrom(ctx)
	if !ok {
		return ids.UserID{}, errs.New(errs.CodeUnauthorized, op)
	}
	if actor.Kind != auth.ActorUser {
		return ids.UserID{}, errs.New(errs.CodeForbidden, op, slog.String("actor_kind", string(actor.Kind)))
	}
	user, err := ids.ParseUserID(actor.ID)
	if err != nil {
		return ids.UserID{}, errs.Wrap(err, errs.CodeUnauthorized, op)
	}
	return user, nil
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
