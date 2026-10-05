package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	governanceport "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type RetryTrade struct {
	SwapID  ids.SwapID
	ActorID ids.UserID
}

type RetryTradeHandler struct {
	uow       *db.UnitOfWork
	reads     sqlc.DBTX
	cabals    Cabals
	proposals Proposals
}

func NewRetryTradeHandler(uow *db.UnitOfWork, reads sqlc.DBTX, cabals Cabals, proposals Proposals) *RetryTradeHandler {
	return &RetryTradeHandler{uow: uow, reads: reads, cabals: cabals, proposals: proposals}
}

func (h *RetryTradeHandler) Handle(ctx context.Context, cmd RetryTrade) error {
	ev, err := h.check(ctx, cmd)
	if err != nil {
		return err
	}
	if err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, ev)
	}); err != nil {
		return err
	}
	observability.Info(ctx, observability.TradingRetryRequested,
		slog.String("swap_id", cmd.SwapID.String()), slog.String("cabal_id", ev.CabalID.String()),
		slog.String("proposal_id", ev.Source.ID.String()), slog.String("requested_by", cmd.ActorID.String()))
	return nil
}

func (h *RetryTradeHandler) check(ctx context.Context, cmd RetryTrade) (events.TradeRetryRequested, error) {
	const op = "trading.RetryTrade"
	row, err := sqlc.New(h.reads).SwapForRetry(ctx, cmd.SwapID.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return events.TradeRetryRequested{}, errs.New(errs.CodeSwapNotFound, op)
	case err != nil:
		return events.TradeRetryRequested{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	member, err := h.cabals.IsMember(ctx, ids.CabalIDFrom(row.CabalID), cmd.ActorID)
	switch {
	case err != nil:
		return events.TradeRetryRequested{}, err
	case !member:
		return events.TradeRetryRequested{}, errs.New(errs.CodeNotCabalMember, op)
	case row.SourceKind != string(domain.SourceProposal) || !row.Retryable:
		return events.TradeRetryRequested{}, errs.New(errs.CodeSwapNotRetryable, op,
			slog.String("source_kind", row.SourceKind), slog.Bool("retryable", row.Retryable))
	}
	status, err := h.proposals.Status(ctx, ids.ProposalIDFrom(row.SourceID))
	switch {
	case err != nil:
		return events.TradeRetryRequested{}, err
	case status != governanceport.StatusPassed:
		return events.TradeRetryRequested{}, errs.New(errs.CodeSwapNotRetryable, op,
			slog.String("proposal_status", string(status)))
	}
	in, inErr := domain.ParseAmount(row.InAmount)
	quote, quoteErr := domain.ParseAmount(row.QuoteOutAmount.Int64)
	if err := errors.Join(inErr, quoteErr); err != nil {
		return events.TradeRetryRequested{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return events.TradeRetryRequested{
		V: 1, SwapID: row.ID, CabalID: row.CabalID,
		Source: events.TradeSource{Kind: row.SourceKind, ID: row.SourceID},
		Action: row.Action, Symbol: row.Symbol,
		InMint: chain.SolanaAddress(row.InMint), OutMint: chain.SolanaAddress(row.OutMint),
		InAmount: in, QuoteOutAmount: quote, SlippageBps: row.SlippageBps, RequestedBy: cmd.ActorID.UUID(),
	}, nil
}
