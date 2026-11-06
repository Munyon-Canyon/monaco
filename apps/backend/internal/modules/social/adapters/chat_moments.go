package adapters

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type ChatMoments struct {
	Assets  app.Assets
	Tallies app.Tallies
	Reads   sqlc.DBTX
	Write   *app.WriteChatMomentHandler
}

type FilledFetch struct {
	Asset    app.AssetCard
	Tallied  bool
	Yes, No  int
	Proposer ids.UserID
}

type SettledFetch struct {
	Skip    bool
	Settled domain.ProposalSettled
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

func (h ChatMoments) FetchFilled(ctx context.Context, e events.TradeConfirmed) (FilledFetch, error) {
	if !tradeSide(e) {
		return FilledFetch{}, nil
	}
	mint, _ := tradeLeg(e)
	asset, err := h.Assets.AssetByMint(ctx, string(mint))
	if errs.CodeOf(err) == errs.CodeAssetNotFound {
		return FilledFetch{}, nil
	}
	if err != nil {
		return FilledFetch{}, err
	}
	fetched := FilledFetch{Asset: asset}
	if e.Source.Kind != proposalSource {
		return fetched, nil
	}
	closed, err := Feed{Tallies: h.Tallies}.FetchClosed(ctx, e.Source.ID)
	if err != nil {
		return FilledFetch{}, err
	}
	if !closed.Skip {
		fetched.Tallied, fetched.Yes, fetched.No = true, closed.Tally.Yes, closed.Tally.No
	}
	if fetched.Proposer, err = h.proposerOf(ctx, e.Source.ID); err != nil {
		return FilledFetch{}, err
	}
	return fetched, nil
}

func (h ChatMoments) proposerOf(ctx context.Context, proposalID uuid.UUID) (ids.UserID, error) {
	const op = "social.ChatMoments.proposerOf"
	author, found, err := h.cardAuthor(ctx, proposalID)
	if err != nil || found {
		return author, err
	}
	source, err := sqlc.New(h.Reads).GetProposalFeedSource(ctx, proposalID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ids.UserID{}, nil
	case err != nil:
		return ids.UserID{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return ids.UserIDFrom(source.ActorID.Bytes), nil
}

func (h ChatMoments) cardAuthor(ctx context.Context, proposalID uuid.UUID) (ids.UserID, bool, error) {
	author, err := sqlc.New(h.Reads).GetProposalCardAuthor(ctx, proposalID)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return ids.UserID{}, false, nil
	case err != nil:
		return ids.UserID{}, false, errs.Wrap(err, errs.CodeInternal, "social.ChatMoments.cardAuthor")
	}
	return ids.UserIDFrom(author), true, nil
}

func (h ChatMoments) FetchFailed(ctx context.Context, e events.ProposalFailed) (SettledFetch, error) {
	return h.fetchSettled(ctx, e.ProposalID, e.Kind, domain.SettledFailed)
}

func (h ChatMoments) FetchExpired(ctx context.Context, e events.ProposalExpired) (SettledFetch, error) {
	return h.fetchSettled(ctx, e.ProposalID, e.Kind, domain.SettledExpired)
}

func (h ChatMoments) fetchSettled(
	ctx context.Context, proposalID uuid.UUID, kind string, outcome domain.SettledOutcome,
) (SettledFetch, error) {
	const op = "social.ChatMoments.fetchSettled"
	if kind == ruleProposal {
		return SettledFetch{Skip: true}, nil
	}
	closed, err := Feed{Tallies: h.Tallies}.FetchClosed(ctx, proposalID)
	if err != nil || closed.Skip {
		return SettledFetch{Skip: true}, err
	}
	source, err := sqlc.New(h.Reads).GetProposalFeedSource(ctx, proposalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return SettledFetch{}, errs.New(errs.CodeFeedItemPending, op)
	}
	if err != nil {
		return SettledFetch{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	payload, err := feed.ParsePayload(source.Payload)
	if err != nil {
		return SettledFetch{}, err
	}
	proposer, found, err := h.cardAuthor(ctx, proposalID)
	if err != nil {
		return SettledFetch{}, err
	}
	if !found {
		proposer = ids.UserIDFrom(source.ActorID.Bytes)
	}
	usdc := payload.USDCMicros
	if usdc.IsZero() {
		usdc = payload.QuoteMicros
	}
	tokens, _ := payload.TokenAmount.Int64()
	side := domain.TradeSide(kind)
	if side != domain.TradeBuy && side != domain.TradeSell {
		side = domain.TradeSide(payload.Action)
	}
	return SettledFetch{Settled: domain.ProposalSettled{
		Outcome: outcome, ProposalID: proposalID, Side: side, Symbol: payload.Symbol,
		USDCMicros: clampInt64(usdc.Uint64()), TokenAmount: tokens,
		Yes: closed.Tally.Yes, No: closed.Tally.No, Voters: closed.Tally.Voters, ProposerID: proposer,
	}}, nil
}

func clampInt64(v uint64) int64 {
	if v > math.MaxInt64 {
		return math.MaxInt64
	}
	return int64(v)
}

func (h ChatMoments) ApplyFailed(
	ctx context.Context, tx db.Tx, e events.ProposalFailed, fetched SettledFetch, at time.Time,
) error {
	return h.applySettled(ctx, tx, e.CabalID, fetched, at)
}

func (h ChatMoments) ApplyExpired(
	ctx context.Context, tx db.Tx, e events.ProposalExpired, fetched SettledFetch, at time.Time,
) error {
	return h.applySettled(ctx, tx, e.CabalID, fetched, at)
}

func (h ChatMoments) applySettled(
	ctx context.Context, tx db.Tx, cabal uuid.UUID, fetched SettledFetch, at time.Time,
) error {
	if fetched.Skip {
		return nil
	}
	moment, err := domain.NewProposalSettledMoment(fetched.Settled)
	if err != nil {
		return err
	}
	_, err = h.Write.In(ctx, tx, app.WriteChatMoment{
		CabalID: ids.CabalIDFrom(cabal), Moment: moment, At: at,
		DedupeKey: "settled:" + fetched.Settled.ProposalID.String(),
	})
	return err
}

func (h ChatMoments) ApplyFilled(
	ctx context.Context, tx db.Tx, e events.TradeConfirmed, fetched FilledFetch, _ time.Time,
) error {
	if !tradeSide(e) {
		return nil
	}
	asset := fetched.Asset
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
		filled.Tallied, filled.Yes, filled.No, filled.ProposerID = fetched.Tallied, fetched.Yes, fetched.No, fetched.Proposer
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
