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
	Boards    app.Boards
	Cabals    app.CabalCheck
	Pages     app.PageLoader
	Follows   app.Follows
	Snapshots app.SnapshotReads
	Ledger    app.Contributions
	Histories app.HistoryLoader
	Stakes    app.StakeHistory
	Cards     app.CabalCards
	Users     app.Users
	Members   app.MemberCabals
}

var _ api.StrictServerInterface = HTTP{}

func (h HTTP) GetCabalsLeaderboard(
	ctx context.Context, req api.GetCabalsLeaderboardRequestObject,
) (api.GetCabalsLeaderboardResponseObject, error) {
	board := boardRef{key: string(domain.BoardCabals), kind: api.Cabal}
	page, err := h.serve(ctx, board, textOf(req.Params.Range), req.Params.Cursor, req.Params.Limit)
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
	filter := domain.FilterAll
	if req.Params.Filter != nil {
		if filter, err = domain.ParseFilter(string(*req.Params.Filter)); err != nil {
			return nil, err
		}
	}
	board := boardRef{
		key: string(domain.BoardPeople), kind: api.User, viewer: &viewer, friends: filter == domain.FilterFriends,
	}
	page, err := h.serve(ctx, board, textOf(req.Params.Range), req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetPeopleLeaderboard200JSONResponse(page), nil
}

func (h HTTP) GetCabalLeaderboard(
	ctx context.Context, req api.GetCabalLeaderboardRequestObject,
) (api.GetCabalLeaderboardResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.Cabals(ctx, ids.CabalIDFrom(req.Id)); err != nil {
		return nil, err
	}
	board := boardRef{key: domain.MembersBoard(req.Id), kind: api.User, viewer: &viewer, unrankedMe: true}
	page, err := h.serve(ctx, board, textOf(req.Params.Range), req.Params.Cursor, req.Params.Limit)
	if err != nil {
		return nil, err
	}
	return api.GetCabalLeaderboard200JSONResponse(page), nil
}

func (h HTTP) GetCabalValueHistory(
	ctx context.Context, req api.GetCabalValueHistoryRequestObject,
) (api.GetCabalValueHistoryResponseObject, error) {
	if _, err := caller(ctx); err != nil {
		return nil, err
	}
	rng := domain.RangeAll
	if req.Params.Range != nil {
		var err error
		if rng, err = domain.ParseRange(string(*req.Params.Range)); err != nil {
			return nil, err
		}
	}
	in := app.ReadValueHistory{
		Cabal: ids.CabalIDFrom(req.Id), Range: rng, Check: h.Cabals, Histories: h.Histories,
	}
	history, err := in.Run(ctx, h.Boards, h.Snapshots, h.Ledger)
	if err != nil {
		return nil, err
	}
	out, err := wireHistory(history, in)
	if err != nil {
		return nil, err
	}
	return api.GetCabalValueHistory200JSONResponse(out), nil
}

func (h HTTP) GetMyPnlHistory(
	ctx context.Context, req api.GetMyPnlHistoryRequestObject,
) (api.GetMyPnlHistoryResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	rng := domain.RangeAll
	if req.Params.Range != nil {
		if rng, err = domain.ParseRange(string(*req.Params.Range)); err != nil {
			return nil, err
		}
	}
	points, err := app.ReadPnLHistory{User: user, Range: rng}.Run(ctx, h.Boards, h.Snapshots, h.Stakes)
	if err != nil {
		return nil, err
	}
	out := api.MyPnlHistory{Range: api.MyPnlHistoryRange(rng), Points: make([]api.PnlPoint, 0, len(points))}
	for _, p := range points {
		equity := p.Equity.Uint64()
		if equity > math.MaxInt64 {
			return nil, errs.New(errs.CodeInternal, "ranking.GetMyPnlHistory", slog.Uint64("equity", equity))
		}
		out.Points = append(out.Points, api.PnlPoint{At: p.At, EquityMicros: int64(equity), PnlMicros: p.PnL.Int64()})
	}
	return api.GetMyPnlHistory200JSONResponse(out), nil
}

func (h HTTP) GetMyPortfolio(
	ctx context.Context, _ api.GetMyPortfolioRequestObject,
) (api.GetMyPortfolioResponseObject, error) {
	user, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := app.ReadPortfolio{User: user}.Run(ctx, h.Boards, h.Snapshots, h.Stakes, h.Cards)
	if err != nil {
		return nil, err
	}
	out, err := wirePortfolio(view)
	if err != nil {
		return nil, err
	}
	return api.GetMyPortfolio200JSONResponse(out), nil
}

func (h HTTP) GetSharedCabals(
	ctx context.Context, req api.GetSharedCabalsRequestObject,
) (api.GetSharedCabalsResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	ports := app.SharedPorts{Users: h.Users, Members: h.Members, Latest: h.Snapshots, Ledger: h.Ledger, Cards: h.Cards}
	rows, err := app.ReadSharedCabals{Viewer: viewer, Other: ids.UserIDFrom(req.Id)}.Run(ctx, ports)
	if err != nil {
		return nil, err
	}
	out := api.SharedCabals{Cabals: make([]api.SharedCabal, 0, len(rows))}
	for _, row := range rows {
		value := row.Value.Uint64()
		if value > math.MaxInt64 {
			return nil, errs.New(errs.CodeInternal, "ranking.GetSharedCabals", slog.Uint64("value", value))
		}
		out.Cabals = append(out.Cabals, api.SharedCabal{
			Cabal: cabalRef(
				row.Cabal,
			),
			ValueMicros: int64(value),
			PnlMicros:   row.PnL.Int64(),
			ReturnBps:   bpsOf(row.Return),
		})
	}
	return api.GetSharedCabals200JSONResponse(out), nil
}

func cabalRef(card app.CabalView) api.CabalRef {
	ref := api.CabalRef{Id: card.ID.UUID(), Name: card.Name}
	if card.PictureURL != "" {
		ref.PictureUrl = &card.PictureURL
	}
	return ref
}

func wirePortfolio(view app.PortfolioView) (api.MyPortfolio, error) {
	const op = "ranking.wirePortfolio"
	total := view.Total.Uint64()
	if total > math.MaxInt64 {
		return api.MyPortfolio{}, errs.New(errs.CodeInternal, op, slog.Uint64("total", total))
	}
	out := api.MyPortfolio{
		TotalValueMicros: int64(total), PnlMicros: view.PnL.Int64(), ReturnBps: bpsOf(view.Return),
		ComputedAt: view.ComputedAt, PricesAsOf: view.PricesAsOf, Cabals: make([]api.PortfolioCabal, 0, len(view.Rows)),
	}
	for _, row := range view.Rows {
		value, shares := row.Value.Uint64(), row.Shares.Uint64()
		if value > math.MaxInt64 || shares > math.MaxInt64 || row.SliceBps > math.MaxInt64 {
			return api.MyPortfolio{}, errs.New(
				errs.CodeInternal, op, slog.Uint64("value", value), slog.Uint64("shares", shares))
		}
		card := view.Cabals[row.CabalID]
		card.ID = row.CabalID
		ref := cabalRef(card)
		out.Cabals = append(out.Cabals, api.PortfolioCabal{
			Cabal: ref, ValueMicros: int64(value), ShareUnits: int64(shares), NetContributedMicros: row.Net.Int64(),
			PnlMicros: row.PnL.Int64(), ReturnBps: bpsOf(row.Return), SliceBps: int64(row.SliceBps),
		})
	}
	return out, nil
}

func bpsOf(b *domain.Bps) *int64 {
	if b == nil {
		return nil
	}
	bps := int64(*b)
	return &bps
}

func wireHistory(history domain.ValueHistory, in app.ReadValueHistory) (api.CabalValueHistory, error) {
	points := make([]api.CabalValuePoint, 0, len(history.Points))
	for _, p := range history.Points {
		value, nav := p.Value.Uint64(), p.NavPerShare.Uint64()
		if value > math.MaxInt64 || nav > math.MaxInt64 {
			return api.CabalValueHistory{}, errs.New(
				errs.CodeInternal, "ranking.wireHistory", slog.Uint64("value", value), slog.Uint64("nav", nav))
		}
		points = append(points, api.CabalValuePoint{
			At: p.At, ValueMicros: int64(value), NavPerShareMicros: int64(nav), PnlMicros: p.PnL.Int64(),
		})
	}
	return api.CabalValueHistory{
		CabalId: in.Cabal.UUID(), Range: api.CabalValueHistoryRange(in.Range), Points: points,
		PricesAsOf: history.PricesAsOf,
	}, nil
}

type boardRef struct {
	key        string
	kind       api.LeaderboardSubjectKind
	viewer     *ids.UserID
	unrankedMe bool
	friends    bool
}

func (h HTTP) serve(
	ctx context.Context, board boardRef, rng, cursor *string, limit *int,
) (api.LeaderboardPage, error) {
	in, err := readOf(board.key, rng, cursor, limit)
	if err != nil {
		return api.LeaderboardPage{}, err
	}
	in.Pages = h.Pages
	if board.friends {
		in.Friends = h.Follows
	}
	if board.viewer != nil {
		id := board.viewer.UUID()
		in.Viewer = &id
	}
	page, err := in.Run(ctx, h.Boards)
	if err != nil {
		return api.LeaderboardPage{}, err
	}
	if page.Me != nil && page.Me.Return == nil && !board.unrankedMe {
		page.Me = nil
	}
	return wirePage(page, in, board.kind)
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
	if page.Me != nil {
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
