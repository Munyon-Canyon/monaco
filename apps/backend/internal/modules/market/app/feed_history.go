package app

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type FeedHistory struct {
	book *PriceBook
	q    *sqlc.Queries
}

func NewFeedHistory(db sqlc.DBTX, c clock.Clock) *FeedHistory {
	return &FeedHistory{book: NewPriceBook(db, c), q: sqlc.New(db)}
}

var _ port.History = (*FeedHistory)(nil)

func (h *FeedHistory) Latest(ctx context.Context, assetIDs []uuid.UUID) (map[uuid.UUID]money.Micros, error) {
	out := make(map[uuid.UUID]money.Micros, len(assetIDs))
	if len(assetIDs) == 0 {
		return out, nil
	}
	ids := make([]domain.AssetID, len(assetIDs))
	for i, id := range assetIDs {
		ids[i] = domain.AssetIDFrom(id)
	}
	recent, err := h.book.SamplesOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	for id, sample := range acceptAll(recent) {
		out[id.UUID()] = sample.Micros
	}
	return out, nil
}

func (h *FeedHistory) Closes(
	ctx context.Context, windows []port.Window, buckets int,
) (map[uuid.UUID][]port.Bucketed, error) {
	out := make(map[uuid.UUID][]port.Bucketed, len(windows))
	if len(windows) == 0 || buckets < 1 {
		return out, nil
	}
	arg := sqlc.WindowClosesParams{Buckets: int32(min(buckets, 1<<30))}
	for _, w := range windows {
		arg.Keys = append(arg.Keys, w.Key)
		arg.AssetIds = append(arg.AssetIds, w.AssetID)
		arg.FromTs = append(arg.FromTs, w.From)
		arg.ToTs = append(arg.ToTs, w.To)
	}
	rows, err := h.q.WindowCloses(ctx, arg)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.FeedHistory.Closes")
	}
	for _, row := range rows {
		micros, err := money.SignedMicrosFromInt64(row.PriceMicros).Micros()
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeDecodeFailed, "market.FeedHistory.Closes",
				slog.String("window", row.WindowKey.String()), slog.Int64("price_micros", row.PriceMicros))
		}
		out[row.WindowKey] = append(out[row.WindowKey], port.Bucketed{At: row.Ts, PriceMicros: micros})
	}
	return out, nil
}
