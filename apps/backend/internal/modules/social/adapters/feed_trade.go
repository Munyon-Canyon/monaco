package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (h Feed) FetchTrade(ctx context.Context, e events.TradeConfirmed) (app.AssetCard, error) {
	if e.Source.Kind != proposalSource {
		return app.AssetCard{}, nil
	}
	mint, _ := tradeLeg(e)
	return h.Assets.AssetByMint(ctx, string(mint))
}

func (h Feed) ApplyTrade(
	ctx context.Context, tx db.Tx, e events.TradeConfirmed, asset app.AssetCard, at time.Time,
) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	q := sqlc.New(tx.Queries())
	cabal, err := q.FeedCabalName(ctx, e.CabalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.ApplyTrade")
	}
	if err != nil {
		return err
	}
	symbol := e.Symbol
	if symbol == "" {
		symbol = asset.Symbol
	}
	_, tokens := tradeLeg(e)
	payload := feed.Payload{
		CabalName: cabal, Symbol: symbol, AssetName: asset.Name, Action: feed.Action(e.Action),
		USDCMicros: e.USDCMicros, PriceMicros: feed.FillPrice(e.USDCMicros, tokens, asset.Decimals),
		TokenAmount: tokens, TokenDecimals: asset.Decimals, ProposalID: e.Source.ID,
	}
	if err := q.InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: eventID(ctx), Kind: string(feed.KindTrade), RefType: string(feed.RefSwaps), RefID: e.SwapID,
		CabalID: pgtype.UUID{Bytes: e.CabalID, Valid: true}, CabalName: pgtype.Text{String: cabal, Valid: true},
		AssetID: pgtype.UUID{Bytes: asset.ID, Valid: true}, Symbol: pgtype.Text{String: symbol, Valid: true},
		Title: feed.RenderTitle(feed.KindTrade, payload), Payload: payload.JSON(), At: at,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func tradeLeg(e events.TradeConfirmed) (chain.SolanaAddress, uint64) {
	if e.Action == string(feed.ActionSell) {
		return e.InMint, e.InAmount
	}
	return e.OutMint, e.OutAmount
}
