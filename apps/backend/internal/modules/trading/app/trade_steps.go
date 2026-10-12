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
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const journalOp = "trading.Journal"

type Journal struct {
	UoW   *db.UnitOfWork
	Reads sqlc.DBTX
	Clock clock.Clock
	IDs   ids.Generator
}

func (j Journal) Begin(ctx context.Context, ref domain.StepRef, amount money.Micros) (domain.TradeStep, error) {
	var step domain.TradeStep
	err := j.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		step, err = j.begin(ctx, tx, ref, amount)
		return err
	})
	return step, err
}

func (j Journal) begin(
	ctx context.Context, tx db.Tx, ref domain.StepRef, amount money.Micros,
) (domain.TradeStep, error) {
	if amount.IsZero() {
		return domain.TradeStep{}, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	q := sqlc.New(tx.Queries())
	n, err := q.InsertTradeStep(ctx, sqlc.InsertTradeStepParams{
		ID: j.IDs.NewV7(), TradeID: ref.SwapID.UUID(), Step: string(ref.Step), Network: string(ref.Network),
		AmountMicros: amount.String(), At: j.Clock.Now(),
	})
	if err != nil {
		return domain.TradeStep{}, err
	}
	step, err := readStep(ctx, q, ref.SwapID, ref.Step)
	switch {
	case err != nil:
		return domain.TradeStep{}, err
	case step.Network != ref.Network:
		return domain.TradeStep{}, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)),
			slog.String("network", string(ref.Network)), slog.String("stored_network", string(step.Network)))
	case n == 0:
		return step, nil
	}
	tx.AfterCommit(func(ctx context.Context) { logStep(ctx, ref, "", step, money.Micros{}) })
	return step, tx.Events.Append(ctx, stepChanged(ref, step, money.Micros{}))
}

type stepMove struct {
	to       domain.StepStatus
	actionID string
	reason   string
	fee      money.Micros
	arrival  domain.Arrival
}

func (j Journal) Submitted(ctx context.Context, ref domain.StepRef, actionID string) (bool, error) {
	return j.in(ctx, func(ctx context.Context, tx db.Tx) (bool, error) { return j.submitted(ctx, tx, ref, actionID) })
}

func (j Journal) submitted(ctx context.Context, tx db.Tx, ref domain.StepRef, actionID string) (bool, error) {
	if actionID == "" {
		return false, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	return j.move(ctx, tx, ref, stepMove{to: domain.StepSubmitted, actionID: actionID})
}

func (j Journal) Succeeded(
	ctx context.Context, ref domain.StepRef, fee money.Micros, arrival domain.Arrival,
) (bool, error) {
	return j.in(ctx, func(ctx context.Context, tx db.Tx) (bool, error) {
		return j.succeeded(ctx, tx, ref, fee, arrival)
	})
}

func (j Journal) succeeded(
	ctx context.Context, tx db.Tx, ref domain.StepRef, fee money.Micros, arrival domain.Arrival,
) (bool, error) {
	return j.move(ctx, tx, ref, stepMove{to: domain.StepSucceeded, fee: fee, arrival: arrival})
}

func (j Journal) Arrived(ctx context.Context, ref domain.StepRef, arrival domain.Arrival) (domain.TradeStep, error) {
	var after domain.TradeStep
	err := j.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		before, err := readStep(ctx, q, ref.SwapID, ref.Step)
		fee, arrived, arrivalErr := arrivalColumns(before.Amount, arrival)
		if err = errors.Join(err, arrivalErr); err != nil {
			return err
		}
		n, err := q.RecordTradeStepArrival(ctx, sqlc.RecordTradeStepArrivalParams{
			ArrivedMicros: arrived, ArrivalTx: arrival.TxHash, At: j.Clock.Now(),
			TradeID: ref.SwapID.UUID(), Step: string(ref.Step),
		})
		if err == nil {
			after, err = readStep(ctx, q, ref.SwapID, ref.Step)
		}
		if err != nil || n == 0 {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { logStep(ctx, ref, before.Status, after, fee) })
		return tx.Events.Append(ctx, stepChanged(ref, after, fee))
	})
	return after, err
}

func (j Journal) ArrivalTxs(ctx context.Context, network chain.Network) ([]string, error) {
	return sqlc.New(j.Reads).ArrivalTxs(ctx, string(network))
}

func arrivalColumns(sent money.Micros, arrival domain.Arrival) (money.Micros, int64, error) {
	fee, err := sent.Sub(arrival.Amount)
	arrived, ok := domain.Column(arrival.Amount.Uint64())
	if err != nil || !ok || arrival.Amount.IsZero() || !arrival.Observed() {
		return money.Micros{}, 0, errs.New(errs.CodeInvalidInput, journalOp,
			slog.String("arrived_micros", arrival.Amount.String()), slog.String("amount_micros", sent.String()))
	}
	return fee, arrived, nil
}

func (j Journal) Failed(ctx context.Context, ref domain.StepRef, reason string) (bool, error) {
	return j.in(ctx, func(ctx context.Context, tx db.Tx) (bool, error) { return j.failed(ctx, tx, ref, reason) })
}

func (j Journal) failed(ctx context.Context, tx db.Tx, ref domain.StepRef, reason string) (bool, error) {
	if reason == "" {
		return false, errs.New(errs.CodeInvalidInput, journalOp, slog.String("step", string(ref.Step)))
	}
	return j.move(ctx, tx, ref, stepMove{to: domain.StepFailed, reason: reason})
}

func (j Journal) Step(ctx context.Context, swap ids.SwapID, kind domain.StepKind) (domain.TradeStep, bool, error) {
	step, err := readStep(ctx, sqlc.New(j.Reads), swap, kind)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return domain.TradeStep{}, false, nil
	}
	return step, err == nil, err
}

func (j Journal) in(ctx context.Context, fn func(ctx context.Context, tx db.Tx) (bool, error)) (bool, error) {
	var moved bool
	err := j.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		moved, err = fn(ctx, tx)
		return err
	})
	return moved && err == nil, err
}

func (j Journal) move(ctx context.Context, tx db.Tx, ref domain.StepRef, m stepMove) (bool, error) {
	q := sqlc.New(tx.Queries())
	before, err := readStep(ctx, q, ref.SwapID, ref.Step)
	switch {
	case errs.CodeOf(err) == errs.CodeNotFound:
		return false, nil
	case err != nil:
		return false, err
	case m.fee.Cmp(before.Amount) > 0:
		return false, errs.New(errs.CodeInvalidInput, journalOp,
			slog.String("fee_micros", m.fee.String()), slog.String("amount_micros", before.Amount.String()))
	}
	var arrived int64
	if m.arrival.Observed() {
		fee, column, err := arrivalColumns(before.Amount, m.arrival)
		if err == nil && fee != m.fee {
			err = errs.New(errs.CodeInvalidInput, journalOp,
				slog.String("fee_micros", m.fee.String()), slog.String("arrived_micros", m.arrival.Amount.String()))
		}
		if err != nil {
			return false, err
		}
		arrived = column
	}
	n, err := j.apply(ctx, q, ref, m, arrived)
	if err != nil || n == 0 {
		return false, err
	}
	after := before
	after.Status, after.Arrival = m.to, m.arrival
	if m.actionID != "" {
		after.PrivyActionID = m.actionID
	}
	tx.AfterCommit(func(ctx context.Context) { logStep(ctx, ref, before.Status, after, m.fee) })
	return true, tx.Events.Append(ctx, stepChanged(ref, after, m.fee))
}

func (j Journal) apply(
	ctx context.Context, q *sqlc.Queries, ref domain.StepRef, m stepMove, arrived int64,
) (int64, error) {
	at, trade, kind := j.Clock.Now(), ref.SwapID.UUID(), string(ref.Step)
	if m.to == domain.StepSubmitted {
		return q.MarkTradeStepSubmitted(ctx, sqlc.MarkTradeStepSubmittedParams{
			PrivyActionID: m.actionID, At: at, TradeID: trade, Step: kind,
		})
	}
	return q.FinishTradeStep(ctx, sqlc.FinishTradeStepParams{
		Status: string(m.to), Error: m.reason, At: at, TradeID: trade, Step: kind,
		ArrivedMicros: arrived, ArrivalTx: m.arrival.TxHash,
	})
}

func logStep(
	ctx context.Context,
	ref domain.StepRef,
	before domain.StepStatus,
	step domain.TradeStep,
	fee money.Micros,
) {
	observability.Info(ctx, observability.TradingStepChanged,
		slog.String("swap_id", ref.SwapID.String()), slog.String("cabal_id", ref.CabalID.String()),
		slog.String("step", string(step.Step)), slog.String("network", string(step.Network)),
		slog.String("before_status", string(before)), slog.String("status", string(step.Status)),
		slog.String("amount_micros", step.Amount.String()), slog.String("fee_micros", fee.String()))
}

func stepChanged(ref domain.StepRef, step domain.TradeStep, fee money.Micros) events.TradeStepChanged {
	e := events.TradeStepChanged{
		V: 1, SwapID: ref.SwapID.UUID(), CabalID: ref.CabalID.UUID(),
		Source:          events.TradeSource{Kind: string(step.Source.Kind), ID: step.Source.ID},
		SourceBatchSize: step.SourceBatchSize, Step: string(step.Step), Network: step.Network,
		Status: string(step.Status), PrivyActionID: step.PrivyActionID, AmountMicros: step.Amount, FeeMicros: fee,
	}
	if step.Attempt > 1 {
		e.Attempt = step.Attempt
	}
	return e
}

func readStep(ctx context.Context, q *sqlc.Queries, swap ids.SwapID, kind domain.StepKind) (domain.TradeStep, error) {
	row, err := q.TradeStep(ctx, sqlc.TradeStepParams{TradeID: swap.UUID(), Step: string(kind)})
	if errors.Is(err, sql.ErrNoRows) {
		return domain.TradeStep{}, errs.New(errs.CodeNotFound, journalOp, slog.String("step", string(kind)))
	}
	if err != nil {
		return domain.TradeStep{}, err
	}
	amount, amountErr := money.ParseMicros(row.AmountMicros)
	arrived, arrivedErr := money.ParseMicros(row.ArrivedMicros)
	if err := errors.Join(amountErr, arrivedErr); err != nil {
		return domain.TradeStep{}, err
	}
	return domain.TradeStep{
		ID: row.ID, SwapID: ids.SwapIDFrom(row.TradeID), CabalID: ids.CabalIDFrom(row.CabalID),
		Source:          domain.Source{Kind: domain.SourceKind(row.SourceKind), ID: row.SourceID},
		SourceBatchSize: int(row.SourceBatchSize), Step: domain.StepKind(row.Step),
		Network: chain.Network(row.Network), PrivyActionID: row.PrivyActionID, Amount: amount,
		Status: domain.StepStatus(row.Status), Error: row.Error, Attempt: int(row.Attempt),
		PriorPrivyActionIDs: row.PriorPrivyActionIds, SentAt: row.SentAt, CreatedAt: row.CreatedAt,
		UpdatedAt: row.UpdatedAt,
		Arrival:   domain.Arrival{Amount: arrived, TxHash: row.ArrivalTx},
	}, nil
}
