package app

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"
	"unicode/utf8"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const retryBridgeBackOp = "trading.RetryBridgeBack"

const actionRetryBridgeBack = "retry_bridge_back"

type RetryBridgeBack struct {
	SwapID ids.SwapID
	Actor  string
	Reason string
}

type FailedBridgeBack struct {
	SwapID   ids.SwapID
	CabalID  ids.CabalID
	Source   domain.SourceKind
	Network  chain.Network
	Amount   money.Micros
	Attempt  int
	Error    string
	FailedAt time.Time
}

type RetryBridgeBackHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	ids   ids.Generator
}

func NewRetryBridgeBackHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, g ids.Generator,
) *RetryBridgeBackHandler {
	return &RetryBridgeBackHandler{uow: uow, reads: reads, clock: c, ids: g}
}

func (h *RetryBridgeBackHandler) ListFailed(ctx context.Context) ([]FailedBridgeBack, error) {
	rows, err := sqlc.New(h.reads).ListFailedBridgeBacks(ctx)
	failed := make([]FailedBridgeBack, 0, len(rows))
	for _, r := range rows {
		amount, parseErr := money.ParseMicros(r.AmountMicros)
		err = errors.Join(err, parseErr)
		failed = append(failed, FailedBridgeBack{
			SwapID: ids.SwapIDFrom(r.TradeID), CabalID: ids.CabalIDFrom(r.CabalID),
			Source: domain.SourceKind(r.SourceKind), Network: chain.Network(r.Network), Amount: amount,
			Attempt: int(r.Attempt), Error: r.Error, FailedAt: r.UpdatedAt,
		})
	}
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, retryBridgeBackOp)
	}
	return failed, nil
}

func (h *RetryBridgeBackHandler) Step(ctx context.Context, swap ids.SwapID) (domain.TradeStep, error) {
	return bridgeBackStep(ctx, sqlc.New(h.reads), swap)
}

func (h *RetryBridgeBackHandler) Handle(ctx context.Context, cmd RetryBridgeBack) (domain.TradeStep, error) {
	if n := utf8.RuneCountInString(cmd.Reason); cmd.Actor == "" || n < 1 || n > 500 {
		return domain.TradeStep{}, errs.New(errs.CodeInvalidInput, retryBridgeBackOp)
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	var after domain.TradeStep
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		after, err = h.retryTx(ctx, tx, cmd)
		return err
	})
	if err != nil {
		return domain.TradeStep{}, err
	}
	observability.Info(ctx, observability.TradingBridgeBackRetried,
		slog.String("swap_id", after.SwapID.String()), slog.String("cabal_id", after.CabalID.String()),
		slog.Int("attempt", after.Attempt), slog.String("actor", cmd.Actor), slog.String("reason", cmd.Reason))
	return after, nil
}

func (h *RetryBridgeBackHandler) retryTx(ctx context.Context, tx db.Tx, cmd RetryBridgeBack) (domain.TradeStep, error) {
	q := sqlc.New(tx.Queries())
	err := q.LockStrandedCash(ctx, cmd.SwapID.UUID())
	var before domain.TradeStep
	if err == nil {
		before, err = bridgeBackStep(ctx, q, cmd.SwapID)
	}
	if err != nil {
		return domain.TradeStep{}, err
	}
	at := h.clock.Now()
	n, err := q.RetryTradeStep(ctx, sqlc.RetryTradeStepParams{At: at, TradeID: cmd.SwapID.UUID()})
	if err == nil && n == 0 {
		err = errs.New(errs.CodeSwapNotStuck, retryBridgeBackOp,
			slog.String("swap_id", cmd.SwapID.String()), slog.String("status", string(before.Status)))
	}
	if err != nil {
		return domain.TradeStep{}, err
	}
	after := before
	after.Attempt, after.Status, after.Error = before.Attempt+1, domain.StepPending, ""
	after.PriorPrivyActionID, after.PrivyActionID, after.UpdatedAt = before.PrivyActionID, "", at
	ref := domain.StepRef{SwapID: after.SwapID, CabalID: after.CabalID, Step: after.Step, Network: after.Network}
	err = tx.Events.Append(ctx, stepChanged(ref, after, money.Micros{}))
	if err == nil {
		err = h.record(ctx, q, cmd, before, after.Attempt, at)
	}
	return after, err
}

func (h *RetryBridgeBackHandler) record(
	ctx context.Context, q *sqlc.Queries, cmd RetryBridgeBack, before domain.TradeStep, attempt int, at time.Time,
) error {
	detail, err := json.Marshal(struct {
		Attempt            int    `json:"attempt"`
		PriorPrivyActionID string `json:"prior_privy_action_id"`
		PriorError         string `json:"prior_error"`
	}{attempt, before.PrivyActionID, before.Error})
	return errors.Join(err, q.InsertSwapOperatorAction(ctx, sqlc.InsertSwapOperatorActionParams{
		ID: h.ids.NewV7(), SwapID: cmd.SwapID.UUID(), Action: actionRetryBridgeBack, Actor: cmd.Actor,
		Reason: cmd.Reason, Detail: detail, CreatedAt: at,
	}))
}

func bridgeBackStep(ctx context.Context, q *sqlc.Queries, swap ids.SwapID) (domain.TradeStep, error) {
	step, err := readStep(ctx, q, swap, domain.StepBridgeBack)
	switch {
	case err == nil:
		return step, nil
	case errs.CodeOf(err) == errs.CodeNotFound:
		return domain.TradeStep{}, errs.New(
			errs.CodeSwapNotFound,
			retryBridgeBackOp,
			slog.String("swap_id", swap.String()),
		)
	}
	return domain.TradeStep{}, errs.Wrap(err, errs.CodeInternal, retryBridgeBackOp)
}
