package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const resolveParkedSwapOp = "trading.ResolveParkedSwap"

type ParkedAction string

const (
	ParkedUnpark ParkedAction = "unpark"
	ParkedFail   ParkedAction = "fail_parked"
)

type ResolveParkedSwap struct {
	SwapID ids.SwapID
	Action ParkedAction
	Actor  string
	Reason string
}

type ParkedSwap struct {
	ID         ids.SwapID
	CabalID    ids.CabalID
	Status     domain.Status
	ParkedAt   time.Time
	Code       string
	BridgeOut  string
	Swap       string
	BridgeBack string
}

type ResolveParkedSwapHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	ids   ids.Generator
	hints Hints
}

func NewResolveParkedSwapHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, g ids.Generator, hints Hints,
) *ResolveParkedSwapHandler {
	return &ResolveParkedSwapHandler{uow: uow, reads: reads, clock: c, ids: g, hints: hints}
}

func (h *ResolveParkedSwapHandler) ListParked(ctx context.Context) ([]ParkedSwap, error) {
	rows, err := sqlc.New(h.reads).ListParkedRouted(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, resolveParkedSwapOp)
	}
	parked := make([]ParkedSwap, 0, len(rows))
	for _, r := range rows {
		parked = append(parked, ParkedSwap{
			ID:         ids.SwapIDFrom(r.ID),
			CabalID:    ids.CabalIDFrom(r.CabalID),
			Status:     domain.Status(r.Status),
			ParkedAt:   r.ParkedAt,
			Code:       r.ParkedCode,
			BridgeOut:  r.BridgeOut,
			Swap:       r.SwapStep,
			BridgeBack: r.BridgeBack,
		})
	}
	return parked, nil
}

func (h *ResolveParkedSwapHandler) Parked(ctx context.Context, id ids.SwapID) (ParkedSwap, error) {
	row, err := sqlc.New(h.reads).ParkedRoutedForUpdate(ctx, id.UUID())
	return parkedView(row), parkedFound(err)
}

func (h *ResolveParkedSwapHandler) Handle(ctx context.Context, cmd ResolveParkedSwap) (ParkedSwap, error) {
	if err := cmd.validate(); err != nil {
		return ParkedSwap{}, err
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	var after ParkedSwap
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		after, err = h.resolveTx(ctx, tx, cmd)
		return err
	})
	if err != nil {
		return ParkedSwap{}, err
	}
	return after, nil
}

func (h *ResolveParkedSwapHandler) resolveTx(ctx context.Context, tx db.Tx, cmd ResolveParkedSwap) (ParkedSwap, error) {
	q := sqlc.New(tx.Queries())
	row, err := lockedParked(ctx, q, cmd.SwapID.UUID())
	if err == nil {
		err = resolvable(row, cmd.Action)
	}
	if err != nil {
		return ParkedSwap{}, err
	}
	before, after := parkedView(row), parkedView(row)
	at := h.clock.Now()
	if cmd.Action == ParkedUnpark {
		after.ParkedAt, after.Code = time.Time{}, ""
		err = unpark(ctx, q, row, at)
	} else {
		after.Status = domain.StatusFailed
		err = failParked(ctx, tx, q, row, at)
	}
	if err == nil {
		err = h.record(ctx, q, cmd, before, at)
	}
	tx.AfterCommit(func(ctx context.Context) { h.committed(ctx, cmd, before) })
	return after, err
}

func lockedParked(ctx context.Context, q *sqlc.Queries, id uuid.UUID) (sqlc.ParkedRoutedForUpdateRow, error) {
	var row sqlc.ParkedRoutedForUpdateRow
	err := q.LockStrandedCash(ctx, id)
	if err == nil {
		row, err = q.ParkedRoutedForUpdate(ctx, id)
	}
	return row, parkedFound(err)
}

func (cmd ResolveParkedSwap) validate() error {
	n := utf8.RuneCountInString(cmd.Reason)
	if cmd.Actor == "" || n < 1 || n > 500 || cmd.Action != ParkedUnpark && cmd.Action != ParkedFail {
		return errs.New(errs.CodeInvalidInput, resolveParkedSwapOp)
	}
	return nil
}

func resolvable(row sqlc.ParkedRoutedForUpdateRow, action ParkedAction) error {
	status := domain.Status(row.Status)
	stuck := row.ParkedAt.Valid && (status == domain.StatusCreated || status == domain.StatusSubmitted)
	if action == ParkedUnpark {
		stuck = stuck && row.BridgeBack == "" && !row.Reused
	} else {
		stuck = stuck && status == domain.StatusCreated && !broadcast(row.SwapStep) &&
			row.SwapStep != string(domain.StepSucceeded) && !broadcast(row.BridgeOut)
	}
	if !stuck {
		return errs.New(errs.CodeSwapNotStuck, resolveParkedSwapOp,
			slog.String("swap_id", row.ID.String()), slog.String("action", string(action)))
	}
	return nil
}

func broadcast(step string) bool {
	return step == string(domain.StepPending) || step == string(domain.StepSubmitted)
}

func unpark(ctx context.Context, q *sqlc.Queries, row sqlc.ParkedRoutedForUpdateRow, at time.Time) error {
	_, err := q.UnparkRouted(ctx, sqlc.UnparkRoutedParams{ID: row.ID, At: at})
	return err
}

func failParked(ctx context.Context, tx db.Tx, q *sqlc.Queries, row sqlc.ParkedRoutedForUpdateRow, at time.Time) error {
	req, err := sweepRequest(row.CabalID, row.SourceKind, row.SourceID, row.Action, row.Symbol, row.InMint,
		row.InAmount, row.SourceBatchSize)
	s := failed(req, row.ID, domain.FailureNeverSubmitted, "")
	if err == nil {
		_, err = s.apply(ctx, q, at)
	}
	if err == nil {
		err = tx.Events.Append(ctx, s.event(at))
	}
	return err
}

func (h *ResolveParkedSwapHandler) record(
	ctx context.Context, q *sqlc.Queries, cmd ResolveParkedSwap, before ParkedSwap, at time.Time,
) error {
	detail, err := json.Marshal(struct {
		ParkedCode string    `json:"parked_code"`
		ParkedAt   time.Time `json:"parked_at"`
		Status     string    `json:"status"`
	}{before.Code, before.ParkedAt.UTC(), string(before.Status)})
	return errors.Join(err, q.InsertSwapOperatorAction(ctx, sqlc.InsertSwapOperatorActionParams{
		ID: h.ids.NewV7(), SwapID: cmd.SwapID.UUID(), Action: string(cmd.Action), Actor: cmd.Actor,
		Reason: cmd.Reason, Detail: detail, CreatedAt: at,
	}))
}

func (h *ResolveParkedSwapHandler) committed(ctx context.Context, cmd ResolveParkedSwap, before ParkedSwap) {
	h.hints.PublishHint(ctx, "cabal."+before.CabalID.UUID().String()+".swap_updated",
		fmt.Appendf(nil, `{"swap_id":%q}`, before.ID.UUID()))
	observability.Info(ctx, observability.TradingRoutedSwapResolved,
		slog.String("swap_id", before.ID.UUID().String()),
		slog.String("cabal_id", before.CabalID.UUID().String()),
		slog.String("action", string(cmd.Action)),
		slog.String("actor", cmd.Actor),
		slog.String("reason", cmd.Reason),
		slog.String("parked_code", before.Code),
	)
}

func parkedView(row sqlc.ParkedRoutedForUpdateRow) ParkedSwap {
	return ParkedSwap{
		ID: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID), Status: domain.Status(row.Status),
		ParkedAt: row.ParkedAt.Time, Code: row.ParkedCode.String, BridgeOut: row.BridgeOut, Swap: row.SwapStep,
		BridgeBack: row.BridgeBack,
	}
}

func parkedFound(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, sql.ErrNoRows):
		return errs.New(errs.CodeSwapNotFound, resolveParkedSwapOp)
	}
	return errs.Wrap(err, errs.CodeInternal, resolveParkedSwapOp)
}
