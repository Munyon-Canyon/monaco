package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
)

const defaultQueuePage = 50

type DeadLetterCount struct{ DB sqlc.DBTX }

func (c DeadLetterCount) CountOpen(ctx context.Context) (int, error) {
	n, err := sqlc.New(c.DB).CountOpenDeadLetters(ctx)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeDBUnavailable, "admin.DeadLetterCount.CountOpen")
	}
	return int(n), nil
}

func (h HTTP) GetAdminQueues(
	ctx context.Context, _ api.GetAdminQueuesRequestObject,
) (api.GetAdminQueuesResponseObject, error) {
	counts, err := h.Queues.Counts(ctx)
	if err != nil {
		return nil, err
	}
	body := api.AdminQueues{Queues: make([]api.AdminQueue, len(counts))}
	for i, c := range counts {
		body.Queues[i] = api.AdminQueue{Name: c.Name, Count: int64(c.Count), Href: c.Href}
	}
	return api.GetAdminQueues200JSONResponse(body), nil
}

func (h HTTP) GetStuckTxns(
	ctx context.Context, req api.GetStuckTxnsRequestObject,
) (api.GetStuckTxnsResponseObject, error) {
	olderThan, err := app.ParseOlderThan(derefString(req.Params.OlderThan), app.StuckAfter)
	if err != nil {
		return nil, err
	}
	rows, err := h.Queues.StuckTxns(ctx, olderThan, queuePage(req.Params.Limit))
	if err != nil {
		return nil, err
	}
	body := api.StuckTxns{Items: make([]api.StuckTxn, len(rows))}
	for i, r := range rows {
		body.Items[i] = api.StuckTxn{
			Id:      r.ID.UUID(),
			CabalId: r.CabalID.UUID(),
			Action:  r.Action,
			Symbol:  r.Symbol,
			Status:  r.Status,
			TxSignature: optionalString(
				r.TxSignature,
			),
			CreatedAt:  r.CreatedAt.UTC(),
			AgeSeconds: int64(r.Age.Seconds()),
		}
	}
	return api.GetStuckTxns200JSONResponse(body), nil
}

func (h HTTP) GetUnpublishedEvents(
	ctx context.Context, req api.GetUnpublishedEventsRequestObject,
) (api.GetUnpublishedEventsResponseObject, error) {
	olderThan, err := app.ParseOlderThan(derefString(req.Params.OlderThan), app.UnpublishedAfter)
	if err != nil {
		return nil, err
	}
	rows, err := h.Queues.UnpublishedEvents(ctx, olderThan, queuePage(req.Params.Limit))
	if err != nil {
		return nil, err
	}
	body := api.UnpublishedEvents{Items: make([]api.UnpublishedEvent, len(rows))}
	for i, r := range rows {
		item := api.UnpublishedEvent{
			Id: r.ID, Type: r.Type, CreatedAt: r.CreatedAt.UTC(), AgeSeconds: int64(r.Age.Seconds()),
		}
		item.Aggregate.Type, item.Aggregate.Id = r.AggregateType, r.AggregateID
		body.Items[i] = item
	}
	return api.GetUnpublishedEvents200JSONResponse(body), nil
}

func queuePage(limit *api.QueueLimit) int {
	if limit == nil {
		return defaultQueuePage
	}
	return *limit
}

func derefString[T ~string](s *T) string {
	if s == nil {
		return ""
	}
	return string(*s)
}
