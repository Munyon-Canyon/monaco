package adapters

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const proposalSource = "proposal"

type ProposalCard struct {
	ProposerName string
	Asset        app.AssetCard
}

func (h Feed) FetchProposal(ctx context.Context, e events.ProposalCreated) (ProposalCard, error) {
	asset, err := h.Assets.AssetByMint(ctx, string(e.Mint))
	if err != nil {
		return ProposalCard{}, err
	}
	proposer := ids.UserIDFrom(e.ProposerID)
	cards, err := h.Users.UsersByID(ctx, []ids.UserID{proposer})
	if err != nil {
		return ProposalCard{}, err
	}
	return ProposalCard{ProposerName: feedName(cards[proposer].DisplayName, cards[proposer].Handle), Asset: asset}, nil
}

func (h Feed) ApplyProposal(
	ctx context.Context, tx db.Tx, e events.ProposalCreated, card ProposalCard, at time.Time,
) error {
	q := sqlc.New(tx.Queries())
	cabal, err := q.FeedCabalName(ctx, e.CabalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.ApplyProposal")
	}
	if err != nil {
		return err
	}
	payload := feed.Payload{
		CabalName: cabal, ActorName: card.ProposerName, Symbol: e.Symbol, AssetName: card.Asset.Name,
		Action: feed.Action(e.Kind), USDCMicros: e.USDCMicros, TokenAmount: e.TokenAmount,
		Status: string(feed.StatusOpen), ExpiresAt: e.ExpiresAt,
	}
	if err := q.InsertFeedConsumerItem(ctx, sqlc.InsertFeedConsumerItemParams{
		ID: eventID(ctx), Kind: string(feed.KindProposal), RefType: string(feed.RefProposals), RefID: e.ProposalID,
		CabalID: pgtype.UUID{Bytes: e.CabalID, Valid: true}, CabalName: pgtype.Text{String: cabal, Valid: true},
		ActorID: pgtype.UUID{Bytes: e.ProposerID, Valid: true}, AssetID: pgtype.UUID{Bytes: card.Asset.ID, Valid: true},
		Symbol: pgtype.Text{String: e.Symbol, Valid: true}, Title: feed.RenderTitle(feed.KindProposal, payload),
		Payload: payload.JSON(), Status: pgtype.Text{String: payload.Status, Valid: true}, At: at,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	return nil
}

func (h Feed) ProposalPassed(ctx context.Context, tx db.Tx, e events.ProposalPassed, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusPassed, "", at)
}

func (h Feed) ProposalFailed(ctx context.Context, tx db.Tx, e events.ProposalFailed, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusFailed, "", at)
}

func (h Feed) ProposalExpired(ctx context.Context, tx db.Tx, e events.ProposalExpired, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusExpired, "", at)
}

func (h Feed) ProposalWithdrawn(ctx context.Context, tx db.Tx, e events.ProposalWithdrawn, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusWithdrawn, "", at)
}

func (h Feed) ProposalVoided(ctx context.Context, tx db.Tx, e events.ProposalVoided, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusVoided, "", at)
}

func (h Feed) ProposalExecuted(ctx context.Context, tx db.Tx, e events.ProposalExecuted, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusExecuted, "", at)
}

func (h Feed) ProposalBlocked(ctx context.Context, tx db.Tx, e events.ProposalExecutionBlocked, at time.Time) error {
	return h.advance(ctx, tx, e.ProposalID, feed.StatusExecutionBlocked, string(e.Code), at)
}

func (h Feed) TradeFailed(ctx context.Context, tx db.Tx, e events.TradeFailed, at time.Time) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	return h.advance(ctx, tx, e.Source.ID, feed.StatusExecutionFailed, e.FailureCode, at)
}

func (h Feed) advance(
	ctx context.Context, tx db.Tx, proposal uuid.UUID, to feed.ProposalStatus, code string, at time.Time,
) error {
	row, err := sqlc.New(tx.Queries()).AdvanceFeedProposal(ctx, sqlc.AdvanceFeedProposalParams{
		ToStatus: string(
			to,
		),
		Patch:        feed.StatusPatch(to, code),
		At:           at,
		ProposalID:   proposal,
		FromStatuses: to.MovesFrom(),
	})
	if err != nil {
		return err
	}
	if !row.Found {
		return errs.New(errs.CodeFeedItemPending, "social.Feed.advance")
	}
	if row.Moved > 0 {
		tx.AfterCommit(func(ctx context.Context) { h.Bus.PublishHint(ctx, "global.feed", nil) })
	}
	return nil
}
