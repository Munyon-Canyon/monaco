package adapters

import (
	"context"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	outcomePassed  = "passed"
	outcomeFailed  = "failed"
	outcomeExpired = "expired"
	voidWithdrawn  = "withdrawn"
)

type Calls struct{}

func (Calls) ApplyCreated(
	ctx context.Context, tx db.Tx, e events.ProposalCreated, opening app.CallOpening, at time.Time,
) error {
	if _, ok := domain.ParseKind(e.Kind); !ok {
		return nil
	}
	q := sqlc.New(tx.Queries())
	asset := pgtype.UUID{}
	if opening.AssetID != nil {
		asset = pgtype.UUID{Bytes: *opening.AssetID, Valid: true}
	}
	err := q.UpsertCallOpened(ctx, sqlc.UpsertCallOpenedParams{
		ProposalID: e.ProposalID, CabalID: e.CabalID, Kind: text(e.Kind), Symbol: text(e.Symbol),
		Mint: text(string(e.Mint)), AssetID: asset, OpenedAt: timestamptz(at), At: at,
	})
	if err != nil {
		return err
	}
	return upsertVoters(ctx, q, e.ProposalID, e.CabalID, opening.Ballots)
}

func (Calls) VoteCast(ctx context.Context, tx db.Tx, e events.VoteCast, _ time.Time) error {
	return sqlc.New(tx.Queries()).UpsertCallVoter(ctx, sqlc.UpsertCallVoterParams{
		ProposalID: e.ProposalID, UserID: e.VoterID, CabalID: e.CabalID, Choice: text(e.Choice),
		ChoiceAt: timestamptz(e.CastAt),
	})
}

func (Calls) ApplyPassed(
	ctx context.Context, tx db.Tx, e events.ProposalPassed, d app.CallDecision, at time.Time,
) error {
	if _, ok := domain.ParseKind(e.Kind); !ok {
		return nil
	}
	return decide(ctx, tx, decided{e.ProposalID, e.CabalID, outcomePassed, domain.SideYes}, d, at)
}

func (Calls) ApplyFailed(
	ctx context.Context, tx db.Tx, e events.ProposalFailed, d app.CallDecision, at time.Time,
) error {
	return decide(ctx, tx, decided{e.ProposalID, e.CabalID, outcomeFailed, domain.SideNo}, d, at)
}

func (Calls) ApplyExpired(
	ctx context.Context, tx db.Tx, e events.ProposalExpired, d app.CallDecision, at time.Time,
) error {
	return decide(ctx, tx, decided{e.ProposalID, e.CabalID, outcomeExpired, domain.SideNo}, d, at)
}

func (Calls) ApplyFill(ctx context.Context, tx db.Tx, e events.TradeConfirmed, f app.CallFill, at time.Time) error {
	if f.Price.IsZero() {
		return nil
	}
	price, err := int8Micros(f.Price)
	if err != nil {
		return err
	}
	return sqlc.New(tx.Queries()).RecordCallFill(ctx, sqlc.RecordCallFillParams{
		ProposalID: e.Source.ID, CabalID: e.CabalID, FillPriceMicros: price, FilledAt: timestamptz(e.ConfirmedAt),
		At: at,
	})
}

func (Calls) Withdrawn(ctx context.Context, tx db.Tx, e events.ProposalWithdrawn, at time.Time) error {
	return voidCall(ctx, tx, e.ProposalID, e.CabalID, voidWithdrawn, at)
}

func (Calls) Voided(ctx context.Context, tx db.Tx, e events.ProposalVoided, at time.Time) error {
	return voidCall(ctx, tx, e.ProposalID, e.CabalID, e.Reason, at)
}

func voidCall(ctx context.Context, tx db.Tx, proposal, cabal uuid.UUID, reason string, at time.Time) error {
	return sqlc.New(tx.Queries()).VoidCall(ctx, sqlc.VoidCallParams{
		ProposalID: proposal, CabalID: cabal, Reason: text(reason), At: at,
	})
}

type decided struct {
	proposal, cabal uuid.UUID
	outcome         string
	side            domain.Side
}

func decide(ctx context.Context, tx db.Tx, d decided, evidence app.CallDecision, at time.Time) error {
	q := sqlc.New(tx.Queries())
	if err := upsertVoters(ctx, q, d.proposal, d.cabal, evidence.Ballots); err != nil {
		return err
	}
	decision := sqlc.RecordCallDecisionParams{
		ProposalID: d.proposal, CabalID: d.cabal, DecidedAt: at, Outcome: text(d.outcome),
	}
	if evidence.Ref != nil {
		ref, err := int8Micros(evidence.Ref.Micros)
		if err != nil {
			return err
		}
		decision.MarketRefMicros = ref
		decision.MarketRefObservedAt = timestamptz(evidence.Ref.ObservedAt)
	}
	if _, err := q.RecordCallDecision(ctx, decision); err != nil {
		return err
	}
	return q.RecordCabalCall(ctx, sqlc.RecordCabalCallParams{
		ProposalID: d.proposal, CabalID: d.cabal, Side: string(d.side),
	})
}

func upsertVoters(ctx context.Context, q *sqlc.Queries, proposal, cabal uuid.UUID, ballots []app.Ballot) error {
	for _, b := range ballots {
		voter := sqlc.UpsertCallVoterParams{ProposalID: proposal, UserID: b.VoterID.UUID(), CabalID: cabal}
		if b.Choice != nil {
			voter.Choice = text(*b.Choice)
		}
		if b.CastAt != nil {
			voter.ChoiceAt = timestamptz(*b.CastAt)
		}
		if err := q.UpsertCallVoter(ctx, voter); err != nil {
			return err
		}
	}
	return nil
}

func text(s string) pgtype.Text { return pgtype.Text{String: s, Valid: true} }

func timestamptz(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func int8Micros(m money.Micros) (pgtype.Int8, error) {
	v := m.Uint64()
	if v > uint64(math.MaxInt64) {
		return pgtype.Int8{}, errs.New(errs.CodeDecodeFailed, "ranking.int8Micros", slog.Uint64("micros", v))
	}
	return pgtype.Int8{Int64: int64(v), Valid: true}, nil
}
