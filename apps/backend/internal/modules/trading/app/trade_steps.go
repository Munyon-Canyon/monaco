package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const journalOp = "trading.Journal"

type Journal struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	IDs   ids.Generator
}

func (j Journal) Begin(ctx context.Context, ref domain.StepRef, amount money.Micros) (domain.TradeStep, error) {
	if amount.IsZero() {
		return domain.TradeStep{}, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	var step domain.TradeStep
	err := j.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		n, err := q.InsertTradeStep(ctx, sqlc.InsertTradeStepParams{
			ID: j.IDs.NewV7(), TradeID: ref.SwapID.UUID(), Step: string(ref.Step), Network: string(ref.Network),
			AmountMicros: amount.String(), At: j.Clock.Now(),
		})
		if err != nil {
			return err
		}
		step, err = readStep(ctx, q, ref.SwapID, ref.Step)
		if err != nil || n == 0 {
			return err
		}
		return tx.Events.Append(ctx, stepChanged(ref, step, money.Micros{}))
	})
	return step, err
}

func (j Journal) Submitted(ctx context.Context, ref domain.StepRef, actionID string) (bool, error) {
	if actionID == "" {
		return false, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	return j.move(ctx, ref, money.Micros{}, func(q *sqlc.Queries) (int64, error) {
		return q.MarkTradeStepSubmitted(ctx, sqlc.MarkTradeStepSubmittedParams{
			PrivyActionID: actionID, At: j.Clock.Now(), TradeID: ref.SwapID.UUID(), Step: string(ref.Step),
		})
	})
}

func (j Journal) Succeeded(ctx context.Context, ref domain.StepRef, fee money.Micros) (bool, error) {
	return j.move(ctx, ref, fee, func(q *sqlc.Queries) (int64, error) {
		return j.finish(ctx, q, ref, domain.StepSucceeded, "")
	})
}

func (j Journal) Failed(ctx context.Context, ref domain.StepRef, reason string) (bool, error) {
	if reason == "" {
		return false, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	return j.move(ctx, ref, money.Micros{}, func(q *sqlc.Queries) (int64, error) {
		return j.finish(ctx, q, ref, domain.StepFailed, reason)
	})
}

func (j Journal) Step(ctx context.Context, swap ids.SwapID, kind domain.StepKind) (domain.TradeStep, bool, error) {
	step, err := readStep(ctx, sqlc.New(j.Reads), swap, kind)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return domain.TradeStep{}, false, nil
	}
	return step, err == nil, err
}

func (j Journal) finish(
	ctx context.Context, q *sqlc.Queries, ref domain.StepRef, to domain.StepStatus, reason string,
) (int64, error) {
	return q.FinishTradeStep(ctx, sqlc.FinishTradeStepParams{
		Status: string(to), Error: reason, At: j.Clock.Now(), TradeID: ref.SwapID.UUID(), Step: string(ref.Step),
	})
}

func (j Journal) move(
	ctx context.Context, ref domain.StepRef, fee money.Micros, apply func(*sqlc.Queries) (int64, error),
) (bool, error) {
	var moved bool
	err := j.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		n, err := apply(q)
		if err != nil || n == 0 {
			return err
		}
		step, err := readStep(ctx, q, ref.SwapID, ref.Step)
		if err != nil {
			return err
		}
		if fee.Cmp(step.Amount) > 0 {
			return errs.New(errs.CodeInvalidInput, journalOp,
				slog.String("fee_micros", fee.String()), slog.String("amount_micros", step.Amount.String()))
		}
		moved = true
		return tx.Events.Append(ctx, stepChanged(ref, step, fee))
	})
	return moved && err == nil, err
}

func stepChanged(ref domain.StepRef, step domain.TradeStep, fee money.Micros) events.TradeStepChanged {
	return events.TradeStepChanged{
		V: 1, SwapID: ref.SwapID.UUID(), CabalID: ref.CabalID.UUID(), Step: string(step.Step), Network: step.Network,
		Status: string(step.Status), PrivyActionID: step.PrivyActionID, AmountMicros: step.Amount, FeeMicros: fee,
	}
}

func readStep(ctx context.Context, q *sqlc.Queries, swap ids.SwapID, kind domain.StepKind) (domain.TradeStep, error) {
	row, err := q.TradeStep(ctx, sqlc.TradeStepParams{TradeID: swap.UUID(), Step: string(kind)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TradeStep{}, errs.New(errs.CodeNotFound, journalOp, slog.String("step", string(kind)))
	}
	if err != nil {
		return domain.TradeStep{}, err
	}
	amount, err := money.ParseMicros(row.AmountMicros)
	if err != nil {
		return domain.TradeStep{}, err
	}
	return domain.TradeStep{
		ID: row.ID, SwapID: ids.SwapIDFrom(row.TradeID), Step: domain.StepKind(row.Step),
		Network: chain.Network(row.Network), PrivyActionID: row.PrivyActionID, Amount: amount,
		Status: domain.StepStatus(row.Status), Error: row.Error, CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt,
	}, nil
}
