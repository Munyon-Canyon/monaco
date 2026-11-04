package adapters

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ChatMoments struct {
	Assets app.Assets
	Write  *app.WriteChatMomentHandler
}

func (h ChatMoments) Created(ctx context.Context, tx db.Tx, e events.CabalCreated, at time.Time) error {
	_, err := h.Write.In(ctx, tx, app.WriteChatMoment{
		CabalID: ids.CabalIDFrom(e.CabalID), Moment: domain.NewCabalCreatedMoment(), At: at, DedupeKey: "created",
	})
	return err
}

func (h ChatMoments) Joined(ctx context.Context, tx db.Tx, e events.CabalMemberJoined, at time.Time) error {
	moment, err := domain.NewMemberJoinedMoment(ids.UserIDFrom(e.UserID))
	if err != nil {
		observability.Debug(ctx, observability.SocialChatMomentSkipped, slog.String("reason", "nil_user"),
			slog.Any("cause", err))
		return nil
	}
	join := e.RequestID
	if join == uuid.Nil {
		join = uuid.UUID(eventID(ctx))
	}
	_, err = h.Write.In(ctx, tx, app.WriteChatMoment{
		CabalID: ids.CabalIDFrom(e.CabalID), Moment: moment, At: at,
		DedupeKey: "joined:" + e.UserID.String() + ":" + join.String(),
	})
	return err
}

func (h ChatMoments) FetchFilled(ctx context.Context, e events.TradeConfirmed) (app.AssetCard, error) {
	if !tradeSide(e) {
		return app.AssetCard{}, nil
	}
	mint, _ := tradeLeg(e)
	asset, err := h.Assets.AssetByMint(ctx, string(mint))
	if errs.CodeOf(err) == errs.CodeAssetNotFound {
		return app.AssetCard{}, nil
	}
	return asset, err
}

func (h ChatMoments) ApplyFilled(
	ctx context.Context, tx db.Tx, e events.TradeConfirmed, asset app.AssetCard, _ time.Time,
) error {
	if !tradeSide(e) {
		return nil
	}
	const op = "social.ChatMoments.ApplyFilled"
	if asset.ID == uuid.Nil {
		observability.Debug(ctx, observability.SocialChatMomentSkipped, slog.String("reason", "asset_not_found"))
		return nil
	}
	_, tokens := tradeLeg(e)
	shares, err := tokens.Uint64()
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidInput, op)
	}
	usdc, price := e.USDCMicros.Uint64(), money.PricePerUnit(e.USDCMicros, shares, asset.Decimals).Uint64()
	if usdc > math.MaxInt64 || price > math.MaxInt64 {
		return errs.New(errs.CodeInvalidInput, op, slog.String("reason", "usdc"))
	}
	filled := domain.TradeFilled{
		Side: domain.TradeSide(e.Action), Symbol: asset.Symbol, AssetID: asset.ID,
		PriceMicros: int64(price), USDCMicros: int64(usdc), FilledAt: e.ConfirmedAt, SwapID: e.SwapID,
	}
	if e.Source.Kind == proposalSource {
		filled.ProposalID = e.Source.ID
	}
	moment, err := domain.NewTradeFilledMoment(filled)
	if err != nil {
		return err
	}
	_, err = h.Write.In(ctx, tx, app.WriteChatMoment{
		CabalID: ids.CabalIDFrom(e.CabalID), Moment: moment, At: e.ConfirmedAt, DedupeKey: "fill:" + e.SwapID.String(),
	})
	return err
}

func tradeSide(e events.TradeConfirmed) bool {
	return e.Action == string(domain.TradeBuy) || e.Action == string(domain.TradeSell)
}
