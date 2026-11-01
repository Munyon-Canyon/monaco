package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type RetryBlockedTrade struct {
	ProposalID ids.ProposalID
	Reason     string
}

type RetryBlockedTradeHandler struct {
	uow   *db.UnitOfWork
	reads sqlc.DBTX
	clock clock.Clock
	swaps Swaps
	hints Hints
	usdc  chain.SolanaAddress
}

func NewRetryBlockedTradeHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, swaps Swaps, hints Hints, usdc chain.SolanaAddress,
) *RetryBlockedTradeHandler {
	return &RetryBlockedTradeHandler{uow: uow, reads: reads, clock: c, swaps: swaps, hints: hints, usdc: usdc}
}

func (h *RetryBlockedTradeHandler) Handle(ctx context.Context, cmd RetryBlockedTrade) error {
	const op = "governance.RetryBlockedTrade"
	reason := strings.TrimSpace(cmd.Reason)
	if reason == "" {
		return errs.New(errs.CodeInvalidInput, op)
	}
	_, found, err := h.swaps.LatestBySource(ctx, trading.Source{Kind: "proposal", ID: cmd.ProposalID.UUID()})
	switch {
	case err != nil:
		return err
	case found:
		return errs.New(errs.CodeSwapNotRetryable, op, slog.String("has_swap", "true"))
	}
	var cabal uuid.UUID
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).ReopenUnstarted(ctx, sqlc.ReopenUnstartedParams{
			ID: cmd.ProposalID.UUID(), At: h.clock.Now(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return h.notRetryable(ctx, tx, cmd.ProposalID)
		}
		if err != nil {
			return errs.Wrap(err, errs.CodeInternal, op)
		}
		cabal = row.CabalID
		if err := tx.Events.Append(ctx, events.ProposalReopened{
			V: 1, ProposalID: row.ID, CabalID: row.CabalID, Reason: reason,
		}); err != nil {
			return err
		}
		return tx.Events.Append(ctx, h.retryEvent(row))
	})
	if err != nil {
		return err
	}
	h.hints.ProposalUpdated(ctx, ids.CabalIDFrom(cabal), cmd.ProposalID)
	observability.Info(ctx, observability.GovernanceBlockedTradeRetried,
		slog.String("proposal_id", cmd.ProposalID.String()), slog.String("cabal_id", cabal.String()),
		slog.String("reason", reason))
	return nil
}

func (h *RetryBlockedTradeHandler) notRetryable(ctx context.Context, tx db.Tx, id ids.ProposalID) error {
	const op = "governance.RetryBlockedTrade"
	status, err := sqlc.New(tx.Queries()).StatusByID(ctx, id.UUID())
	if err != nil {
		return lookupFailed(err, op)
	}
	return errs.New(errs.CodeSwapNotRetryable, op, slog.String("status", status))
}

func (h *RetryBlockedTradeHandler) retryEvent(row sqlc.ReopenUnstartedRow) events.TradeRetryRequested {
	ev := events.TradeRetryRequested{
		V: 1, CabalID: row.CabalID, Source: events.TradeSource{Kind: "proposal", ID: row.ID}, Action: row.Kind,
		Symbol: row.Symbol, QuoteOutAmount: unsigned(row.QuoteOutAmount), Network: chain.Network(row.Network),
	}
	if row.Kind == "sell" {
		ev.InMint, ev.OutMint, ev.InAmount = chain.SolanaAddress(row.Mint), h.usdc, unsigned(row.TokenAmount)
		return ev
	}
	ev.InMint, ev.OutMint, ev.InAmount = h.usdc, chain.SolanaAddress(row.Mint), unsigned(row.UsdcMicros)
	return ev
}
