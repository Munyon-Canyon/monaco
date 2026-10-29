package adapters

import (
	"context"
	"log/slog"
	"math"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CallBoardHeadCache struct {
	Cache *Cache[app.CallBoardHeadKey, app.CallBoardHead]
}

func NewCallBoardHeadCache(size int) CallBoardHeadCache {
	return CallBoardHeadCache{Cache: NewCache[app.CallBoardHeadKey, app.CallBoardHead](size)}
}

func (c CallBoardHeadCache) Load(
	ctx context.Context, key app.CallBoardHeadKey, load func(context.Context) (app.CallBoardHead, error),
) (app.CallBoardHead, error) {
	head, err := c.Cache.Load(ctx, key, load)
	if err != nil {
		return app.CallBoardHead{}, err
	}
	return *head, nil
}

func (h HTTP) GetCallLeaderboard(
	ctx context.Context, req api.GetCallLeaderboardRequestObject,
) (api.GetCallLeaderboardResponseObject, error) {
	viewer, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	q, err := callBoardQuery(viewer, req.Params)
	if err != nil {
		return nil, err
	}
	view, err := h.CallBoard.Run(ctx, q)
	if err != nil {
		return nil, err
	}
	return api.GetCallLeaderboard200JSONResponse(wireCallBoard(view)), nil
}

func callBoardQuery(viewer ids.UserID, params api.GetCallLeaderboardParams) (app.CallBoardQuery, error) {
	q := app.CallBoardQuery{Viewer: viewer, Board: domain.BoardPeople, Period: domain.PeriodMonth}
	if params.Board != nil {
		q.Board = domain.Board(*params.Board)
	}
	if params.Period != nil {
		q.Period = domain.PeriodKind(*params.Period)
	}
	q.Friends = params.Filter != nil && *params.Filter == api.GetCallLeaderboardParamsFilterFriends
	if q.Friends && q.Board == domain.BoardCabals {
		return q, errs.New(errs.CodeInvalidInput, "ranking.callBoardQuery", slog.String("filter", "friends"))
	}
	if params.Cursor != nil {
		q.Cursor = *params.Cursor
	}
	return q, nil
}

func wireCallBoard(v app.CallBoardView) api.CallBoard {
	filter := api.CallBoardFilterAll
	if v.Friends {
		filter = api.CallBoardFilterFriends
	}
	out := api.CallBoard{
		Board:  api.CallBoardBoard(v.Board),
		Filter: filter,
		Period: api.CallBoardPeriod{
			Kind: api.CallBoardPeriodKind(v.Period), Start: wireDate(v.PeriodStart), End: wireDate(v.PeriodEnd),
		},
		ComputedAt:  v.ComputedAt,
		TotalRanked: clamp32(v.TotalRanked),
		Top:         wireCallRows(v.Top),
		Me:          wireCallRow(v.Me),
		Above:       wireCallRow(v.Above),
		Below:       wireCallRow(v.Below),
		ClimbBps: clamp32(
			v.ClimbBps,
		),
		Around:   api.CallBoardAround{Rows: wireCallRows(v.Around), AfterCursor: v.AfterCursor},
		Unranked: wireCallRows(v.Unranked),
	}
	if v.Gap != nil {
		out.Gap = &api.CallBoardGap{
			Versus: api.CallBoardGapVersus(v.Gap.Versus), Points: clamp32(v.Gap.Points),
			CallsToPass: int32Ptr(v.Gap.CallsToPass), AboveIsPerfect: v.Gap.AboveIsPerfect,
		}
	}
	if l := v.LeadChange; l != nil {
		out.LeadChange = &api.CallBoardLeadChange{
			Id: l.ID, Leader: wireCallSubject(l.Leader), Passed: wireCallSubject(l.Passed),
			LeaderRatePct: clamp32(l.LeaderRatePct), ChangedAt: l.ChangedAt, MovementPlaces: clamp32(l.MovementPlaces),
		}
		if l.Call != nil {
			out.LeadChange.Call = &api.CallBoardLeadCall{
				ProposalId: l.Call.ProposalID,
				Kind:       api.CallBoardLeadChangeCallKind(l.Call.Kind),
				Symbol:     l.Call.Symbol,
			}
		}
	}
	if v.UnrankedMe != nil {
		out.UnrankedMe = &api.CallBoardUnrankedMe{
			Subject: wireCallSubject(v.UnrankedMe.Subject), Resolved: clamp32(v.UnrankedMe.Resolved),
			Needed: clamp32(v.UnrankedMe.Needed), Voted: clamp32(v.UnrankedMe.Voted),
			Eligible: clamp32(v.UnrankedMe.Eligible), PendingSymbols: v.UnrankedMe.PendingSymbols,
		}
	}
	return out
}

func wireDate(t *time.Time) *openapi_types.Date {
	if t == nil {
		return nil
	}
	return &openapi_types.Date{Time: *t}
}

func clamp32(n int) int32 { return int32(min(max(n, math.MinInt32), math.MaxInt32)) }

func int32Ptr(n *int) *int32 {
	if n == nil {
		return nil
	}
	v := clamp32(*n)
	return &v
}

func wireCallRows(rows []app.CallRow) []api.CallBoardRow {
	out := make([]api.CallBoardRow, len(rows))
	for i := range rows {
		out[i] = *wireCallRow(&rows[i])
	}
	return out
}

func wireCallRow(r *app.CallRow) *api.CallBoardRow {
	if r == nil {
		return nil
	}
	row := api.CallBoardRow{
		Rank:       int32Ptr(r.Rank),
		Subject:    wireCallSubject(r.Subject),
		RateBps:    int32Ptr(r.RateBps),
		RatePct:    int32Ptr(r.RatePct),
		Right:      clamp32(r.Right),
		Resolved:   clamp32(r.Resolved),
		Voted:      clamp32(r.Voted),
		Eligible:   clamp32(r.Eligible),
		TurnoutBps: clamp32(r.TurnoutBps),
		Needed:     int32Ptr(r.Needed),
		Movement: api.CallBoardMovement{
			Kind:   api.CallBoardMovementKind(r.Movement.Kind),
			Places: clamp32(r.Movement.Places),
		},
		IsViewer: r.IsViewer,
		IsMine:   r.IsMine,
	}
	if r.LastCall != nil {
		row.LastCall = &api.CallBoardLastCall{
			ProposalId: r.LastCall.ProposalID, Kind: api.CallBoardRowLastCallKind(r.LastCall.Kind),
			Symbol: r.LastCall.Symbol, Right: r.LastCall.Right,
		}
	}
	return &row
}

func wireCallSubject(s app.CallSubjectView) api.LeaderboardSubject {
	kind := api.User
	if s.Cabal {
		kind = api.Cabal
	}
	return api.LeaderboardSubject{Id: s.ID, Kind: kind, Name: s.Name, Handle: s.Handle, PictureUrl: s.PictureURL}
}
