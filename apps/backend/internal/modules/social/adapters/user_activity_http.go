package adapters

import (
	"context"
	"math"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h HTTP) GetUserActivity(
	ctx context.Context, req api.GetUserActivityRequestObject,
) (api.GetUserActivityResponseObject, error) {
	const op = "social.GetUserActivity"
	me, err := caller(ctx)
	if err != nil {
		return nil, err
	}
	limit := app.UserActivityPageDefault
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}
	var cursor *app.ActivityPosition
	if req.Params.Cursor != nil {
		position, err := app.ParseActivityPosition(*req.Params.Cursor)
		if err != nil {
			return nil, err
		}
		cursor = &position
	}
	page, err := h.Activity.Run(ctx, me, ids.UserIDFrom(req.Id), cursor, limit)
	if err != nil {
		return nil, err
	}
	body := api.UserActivityPage{Items: make([]api.UserActivity, len(page.Items))}
	for i, row := range page.Items {
		if err := wireActivity(&body.Items[i], row); err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	if page.Next != nil {
		next := page.Next.Encode()
		body.NextCursor = &next
	}
	return api.GetUserActivity200JSONResponse(body), nil
}

func wireActivity(out *api.UserActivity, row app.ActivityRow) error {
	cabal := api.UserActivityCabal{
		Id: row.Cabal.ID.UUID(), Name: row.Cabal.Name, PictureUrl: optionalWireText(row.Cabal.PictureURL),
	}
	micros := wireMicros(row.USDCMicros)
	var err error
	switch row.Kind {
	case app.ActivityVoted:
		voted := api.UserActivityVoted{
			Kind: api.Voted, At: row.At, Title: "Voted", Cabal: cabal, ProposalId: row.ProposalID.UUID(),
			Subject: row.Subject, Symbol: row.Symbol, UsdcMicros: micros,
		}
		if row.Choice != "" {
			choice := api.UserActivityVotedChoice(row.Choice)
			voted.Choice = &choice
		}
		err = out.FromUserActivityVoted(voted)
	case app.ActivityProposed:
		proposed := api.UserActivityProposed{
			Kind: api.Proposed, At: row.At, Title: "Proposed", Cabal: cabal, ProposalId: row.ProposalID.UUID(),
			Subject: row.Subject, Symbol: row.Symbol, UsdcMicros: micros, Status: row.Status,
		}
		if row.Result != nil {
			proposed.Result = &struct {
				No  int `json:"no"`
				Yes int `json:"yes"`
			}{No: row.Result.No, Yes: row.Result.Yes}
		}
		err = out.FromUserActivityProposed(proposed)
	case app.ActivityReplied:
		err = out.FromUserActivityReplied(api.UserActivityReplied{
			Kind: api.Replied, At: row.At, Cabal: cabal, CommentId: row.CommentID, ProposalId: row.ProposalID.UUID(),
			Body: row.Body, Subject: row.Subject, UsdcMicros: micros,
		})
	case app.ActivityToppedBoard:
		err = out.FromUserActivityToppedBoard(api.UserActivityToppedBoard{
			Kind: api.ToppedBoard, At: row.At, Title: "Topped the board",
			PeriodStart: openapi_types.Date{Time: row.PeriodStart}, Right: row.Right, Resolved: row.Resolved,
		})
	}
	return err
}

func wireMicros(v *uint64) *int64 {
	if v == nil || *v > math.MaxInt64 {
		return nil
	}
	micros := int64(*v)
	return &micros
}
