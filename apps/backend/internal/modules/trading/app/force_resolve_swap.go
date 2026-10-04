package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const forceResolveSwapOp = "trading.ForceResolveSwap"

type ForceResolveSwap struct {
	Signature      chain.Signature
	To             domain.Status
	OutAmount      money.BaseUnits
	Reason         string
	Actor          string
	IdempotencyKey string
}

type ForceResolveSwapHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	hints Hints
}

func NewForceResolveSwapHandler(
	uow *db.UnitOfWork,
	reads sqlc.DBTX,
	c clock.Clock,
	hints Hints,
) *ForceResolveSwapHandler {
	return &ForceResolveSwapHandler{uow: uow, reads: reads, clock: c, hints: hints}
}

func (h *ForceResolveSwapHandler) Handle(ctx context.Context, cmd ForceResolveSwap) (SwapView, error) {
	if err := cmd.validate(); err != nil {
		return SwapView{}, err
	}
	return h.resolve(observability.WithActor(ctx, cmd.Actor), cmd)
}

func (h *ForceResolveSwapHandler) resolve(ctx context.Context, cmd ForceResolveSwap) (SwapView, error) {
	var resolved SwapView
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		resolved, err = h.resolveTx(ctx, tx, cmd)
		return err
	})
	if err != nil {
		return SwapView{}, err
	}
	return resolved, nil
}

func (h *ForceResolveSwapHandler) resolveTx(ctx context.Context, tx db.Tx, cmd ForceResolveSwap) (SwapView, error) {
	row, err := sqlc.New(tx.Queries()).SwapForForceResolve(ctx, string(cmd.Signature))
	if err != nil {
		return SwapView{}, forceResolveFound(err)
	}
	if row.Status != string(domain.StatusSubmitted) {
		return SwapView{}, errs.New(errs.CodeSwapNotStuck, forceResolveSwapOp)
	}
	req, err := sweepRequest(
		row.CabalID,
		row.SourceKind,
		row.SourceID,
		row.Action,
		row.Symbol,
		row.InMint,
		row.InAmount,
		row.SourceBatchSize,
	)
	if err != nil {
		return SwapView{}, err
	}
	if cmd.To == domain.StatusConfirmed {
		if row.OutDecimals < 0 || row.OutDecimals > 255 {
			return SwapView{}, errs.New(errs.CodeDecodeFailed, forceResolveSwapOp)
		}
		if int16(cmd.OutAmount.Decimals()) != row.OutDecimals {
			return SwapView{}, errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
		}
		req.OutMint = chain.Mint{Address: chain.SolanaAddress(row.OutMint), Decimals: uint8(row.OutDecimals)}
	}
	s := cmd.step(req, row.ID)
	at := h.clock.Now()
	n, err := s.apply(ctx, sqlc.New(tx.Queries()), at)
	if err != nil {
		return SwapView{}, err
	}
	if n == 0 {
		return SwapView{}, errs.New(errs.CodeSwapNotStuck, forceResolveSwapOp)
	}
	if err := tx.Events.Append(ctx, s.event(at)); err != nil {
		return SwapView{}, err
	}
	resolved, err := NewQueries(tx.Queries()).SwapBySignature(ctx, cmd.Signature)
	if err != nil {
		return SwapView{}, err
	}
	tx.AfterCommit(func(ctx context.Context) { h.committed(ctx, req, row.ID, cmd, s) })
	return resolved, nil
}

func (cmd ForceResolveSwap) validate() error {
	if cmd.Signature == "" || cmd.IdempotencyKey == "" || cmd.Actor == "" ||
		utf8.RuneCountInString(cmd.Reason) < 1 || utf8.RuneCountInString(cmd.Reason) > 500 {
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	}
	switch cmd.To {
	case domain.StatusConfirmed:
		if cmd.OutAmount.IsZero() {
			return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
		}
		if _, ok := domain.Column(cmd.OutAmount.Uint64()); !ok {
			return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
		}
	case domain.StatusFailed:
	case domain.StatusCreated, domain.StatusSubmitted:
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	default:
		return errs.New(errs.CodeInvalidInput, forceResolveSwapOp)
	}
	return nil
}

func (cmd ForceResolveSwap) step(req SwapRequest, id uuid.UUID) step {
	if cmd.To == domain.StatusConfirmed {
		out, _ := domain.Column(cmd.OutAmount.Uint64())
		return confirmed(req, id, cmd.Signature, cmd.OutAmount.Uint64(), out)
	}
	return failed(req, id, domain.FailureForceResolved, "")
}

func forceResolveFound(err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return errs.New(errs.CodeSwapNotFound, forceResolveSwapOp)
	}
	return errs.Wrap(err, errs.CodeInternal, forceResolveSwapOp)
}

func (h *ForceResolveSwapHandler) committed(
	ctx context.Context,
	req SwapRequest,
	id uuid.UUID,
	cmd ForceResolveSwap,
	s step,
) {
	h.hints.PublishHint(
		ctx,
		"cabal."+req.CabalID.UUID().String()+".swap_updated",
		fmt.Appendf(nil, `{"swap_id":%q}`, id),
	)
	observability.Info(ctx, observability.TradingSwapForceResolved,
		slog.String("swap_id", id.String()),
		slog.String("signature", string(cmd.Signature)),
		slog.String("from", string(s.from)),
		slog.String("to", string(s.status)),
		slog.String("reason", cmd.Reason),
	)
}
