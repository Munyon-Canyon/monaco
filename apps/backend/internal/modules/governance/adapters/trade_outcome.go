package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const proposalSource = "proposal"

type TradeOutcome struct{ Hints app.Hints }

func (h TradeOutcome) Confirmed(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	return settle(ctx, tx, h.Hints, outcome{
		proposal: e.Source.ID, cabal: e.CabalID, to: domain.StatusExecuted, at: at,
		emit: events.ProposalExecuted{V: 1, ProposalID: e.Source.ID, CabalID: e.CabalID, SwapID: e.SwapID},
	})
}

func (h TradeOutcome) Blocked(ctx context.Context, tx db.Tx, e events.TradeBlocked, at time.Time) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	return h.block(ctx, tx, e.Source.ID, e.CabalID, e.Code, at)
}

func (h TradeOutcome) Failed(ctx context.Context, tx db.Tx, e events.TradeFailed, at time.Time) error {
	if e.Source.Kind != proposalSource {
		return nil
	}
	return h.block(ctx, tx, e.Source.ID, e.CabalID, errs.CodeSwapFailed, at)
}

func (h TradeOutcome) block(
	ctx context.Context, tx db.Tx, proposal, cabal uuid.UUID, code errs.Code, at time.Time,
) error {
	return settle(ctx, tx, h.Hints, outcome{
		proposal: proposal, cabal: cabal, to: domain.StatusExecutionBlocked, at: at,
		reason: pgtype.Text{String: string(code), Valid: true},
		emit:   events.ProposalExecutionBlocked{V: 1, ProposalID: proposal, CabalID: cabal, Code: code},
	})
}

func (h TradeOutcome) Retried(ctx context.Context, tx db.Tx, e events.TradeRetryRequested, at time.Time) error {
	const op = "governance.TradeOutcome.Retried"
	if e.Source.Kind != proposalSource {
		return nil
	}
	moved, err := sqlc.New(tx.Queries()).Reopen(ctx, sqlc.ReopenParams{ID: e.Source.ID, At: at})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	if moved == 0 {
		return nil
	}
	if err := tx.Events.Append(ctx, events.ProposalReopened{
		V: 1, ProposalID: e.Source.ID, CabalID: e.CabalID, SwapID: e.SwapID,
	}); err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.Hints.ProposalUpdated(ctx, ids.CabalIDFrom(e.CabalID), ids.ProposalIDFrom(e.Source.ID))
	})
	return nil
}

type outcome struct {
	proposal uuid.UUID
	cabal    uuid.UUID
	to       domain.Status
	reason   pgtype.Text
	at       time.Time
	emit     events.Event
}

func settle(ctx context.Context, tx db.Tx, hints app.Hints, o outcome) error {
	const op = "governance.TradeOutcome"
	q := sqlc.New(tx.Queries())
	moved, err := q.Transition(ctx, sqlc.TransitionParams{
		ID: o.proposal, FromStatus: string(domain.StatusPassed), ToStatus: string(o.to), Reason: o.reason, At: o.at,
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	repeatsFailure := o.to == domain.StatusExecutionBlocked && o.reason.String == string(errs.CodeSwapFailed)
	if moved == 0 && !repeatsFailure {
		moved, err = q.TransitionAfterSwapFailure(ctx, sqlc.TransitionAfterSwapFailureParams{
			ID: o.proposal, ToStatus: string(o.to), Reason: o.reason, At: o.at,
		})
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	if moved == 1 {
		if err := tx.Events.Append(ctx, o.emit); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) {
			hints.ProposalUpdated(ctx, ids.CabalIDFrom(o.cabal), ids.ProposalIDFrom(o.proposal))
		})
		return nil
	}
	status, err := q.StatusByID(ctx, o.proposal)
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	s := domain.Status(status)
	if s == o.to || s == domain.StatusVoided {
		return nil
	}
	if s == domain.StatusOpen {
		return errs.New(errs.CodeProposalStillOpen, op)
	}
	return errs.New(errs.CodeVersionConflict, op, slog.String("status", status), slog.String("to", string(o.to)))
}
