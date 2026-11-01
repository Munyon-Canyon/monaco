package app

import (
	"context"
	"database/sql"
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

const (
	returnEVMCashOp     = "trading.ReturnEVMCash"
	actionReturnEVMCash = "return_evm_cash"
)

type ReturnEVMCash struct {
	SwapID ids.SwapID
	Actor  string
	Reason string
}

type EVMCashReturn struct {
	Requested ids.SwapID
	Holder    ids.SwapID
	CabalID   ids.CabalID
	Network   chain.Network
	Landed    money.Micros
	OnChain   money.Micros
	Amount    money.Micros
	Uncovered money.Micros
}

type EVMCashWriteOff struct {
	SwapID  ids.SwapID
	CabalID ids.CabalID
	Network chain.Network
	Amount  money.Micros
}

type EVMCashWriteOffs interface {
	WriteOff(ctx context.Context, tx db.Tx, w EVMCashWriteOff) error
}

type ReturnEVMCashHandler struct {
	uow     *db.UnitOfWork
	reads   sqlc.DBTX
	clock   clock.Clock
	ids     ids.Generator
	wallets TreasuryEVM
	cash    EVMCash
	offs    EVMCashWriteOffs
}

func NewReturnEVMCashHandler(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, g ids.Generator, wallets TreasuryEVM, cash EVMCash,
	offs EVMCashWriteOffs,
) *ReturnEVMCashHandler {
	return &ReturnEVMCashHandler{uow: uow, reads: reads, clock: c, ids: g, wallets: wallets, cash: cash, offs: offs}
}

func (h *ReturnEVMCashHandler) Plan(ctx context.Context, swap ids.SwapID) (EVMCashReturn, error) {
	q := sqlc.New(h.reads)
	holder, err := q.CashHolder(ctx, swap.UUID())
	var row sqlc.ReturnableCashRow
	if err == nil {
		row, err = q.ReturnableCash(ctx, holder)
	}
	if errors.Is(err, sql.ErrNoRows) {
		return EVMCashReturn{}, errs.New(errs.CodeSwapNotFound, returnEVMCashOp, slog.String("swap_id", swap.String()))
	}
	if err != nil {
		return EVMCashReturn{}, errs.Wrap(err, errs.CodeInternal, returnEVMCashOp)
	}
	plan, err := returnable(swap, row)
	if err != nil {
		return EVMCashReturn{}, err
	}
	wallet, err := h.wallets.EnsureTreasuryEVMWallet(ctx, plan.CabalID)
	if err == nil {
		plan.OnChain, err = h.cash.Balance(ctx, plan.Network, wallet.Address)
	}
	if err == nil {
		plan.Amount = atMost(plan.Landed, plan.OnChain)
		plan.Uncovered, err = plan.Landed.Sub(plan.Amount)
	}
	if err != nil {
		return EVMCashReturn{}, err
	}
	if plan.Amount.IsZero() {
		return EVMCashReturn{}, refuse(errs.CodeInsufficientFunds, row, "evm_balance_zero")
	}
	return plan, nil
}

func (h *ReturnEVMCashHandler) Handle(ctx context.Context, cmd ReturnEVMCash) (EVMCashReturn, error) {
	if n := utf8.RuneCountInString(cmd.Reason); cmd.Actor == "" || n < 1 || n > 500 {
		return EVMCashReturn{}, errs.New(errs.CodeInvalidInput, returnEVMCashOp)
	}
	plan, err := h.Plan(ctx, cmd.SwapID)
	if err != nil {
		return EVMCashReturn{}, err
	}
	ctx = observability.WithActor(ctx, "system:"+cmd.Actor)
	if err := h.uow.Do(
		ctx,
		func(ctx context.Context, tx db.Tx) error { return h.queue(ctx, tx, cmd, plan) },
	); err != nil {
		return EVMCashReturn{}, err
	}
	observability.Info(ctx, observability.TradingEVMCashReturnQueued,
		slog.String("swap_id", plan.Holder.String()), slog.String("requested_swap_id", plan.Requested.String()),
		slog.String("cabal_id", plan.CabalID.String()), slog.String("network", string(plan.Network)),
		slog.String("landed_micros", plan.Landed.String()), slog.String("on_chain_micros", plan.OnChain.String()),
		slog.String("amount_micros", plan.Amount.String()), slog.String("uncovered_micros", plan.Uncovered.String()),
		slog.String("actor", cmd.Actor),
		slog.String("reason", cmd.Reason))
	return plan, nil
}

func (h *ReturnEVMCashHandler) queue(ctx context.Context, tx db.Tx, cmd ReturnEVMCash, plan EVMCashReturn) error {
	q := sqlc.New(tx.Queries())
	err := q.LockStrandedCash(ctx, plan.Holder.UUID())
	var stranded bool
	if err == nil {
		stranded, err = q.StrandedCash(ctx, plan.Holder.UUID())
	}
	if err == nil && !stranded {
		err = errs.New(errs.CodeStrandedCashTaken, returnEVMCashOp,
			slog.String("swap_id", plan.Holder.String()), slog.String("reason", "cash_moved"))
	}
	if err != nil {
		return err
	}
	ref := domain.StepRef{
		SwapID:  plan.Holder,
		CabalID: plan.CabalID,
		Step:    domain.StepBridgeBack,
		Network: plan.Network,
	}
	_, err = (Journal{UoW: h.uow, Clock: h.clock, IDs: h.ids}).begin(ctx, tx, ref, plan.Amount)
	if err == nil {
		err = h.record(ctx, q, cmd, plan, h.clock.Now())
	}
	if err == nil && !plan.Uncovered.IsZero() {
		err = h.offs.WriteOff(ctx, tx, EVMCashWriteOff{
			SwapID: plan.Holder, CabalID: plan.CabalID, Network: plan.Network, Amount: plan.Uncovered,
		})
		tx.AfterCommit(func(ctx context.Context) {
			observability.Alert(ctx, observability.TradingEVMCashUncovered,
				slog.String("swap_id", plan.Holder.String()), slog.String("cabal_id", plan.CabalID.String()),
				slog.String("network", string(plan.Network)), slog.String("landed_micros", plan.Landed.String()),
				slog.String("on_chain_micros", plan.OnChain.String()),
				slog.String("uncovered_micros", plan.Uncovered.String()), slog.String("actor", cmd.Actor),
				slog.String("reason", cmd.Reason))
		})
	}
	return err
}

func (h *ReturnEVMCashHandler) record(
	ctx context.Context, q *sqlc.Queries, cmd ReturnEVMCash, plan EVMCashReturn, at time.Time,
) error {
	detail, err := json.Marshal(struct {
		Requested string `json:"requested_swap_id"`
		Network   string `json:"network"`
		Landed    string `json:"landed_micros"`
		OnChain   string `json:"on_chain_micros"`
		Amount    string `json:"amount_micros"`
		Uncovered string `json:"uncovered_micros"`
	}{
		plan.Requested.String(), string(plan.Network), plan.Landed.String(), plan.OnChain.String(),
		plan.Amount.String(), plan.Uncovered.String(),
	})
	return errors.Join(err, q.InsertSwapOperatorAction(ctx, sqlc.InsertSwapOperatorActionParams{
		ID: h.ids.NewV7(), SwapID: plan.Holder.UUID(), Action: actionReturnEVMCash, Actor: cmd.Actor,
		Reason: cmd.Reason, Detail: detail, CreatedAt: at,
	}))
}

func returnable(requested ids.SwapID, row sqlc.ReturnableCashRow) (EVMCashReturn, error) {
	network := chain.Network(row.Network)
	landed, err := money.ParseMicros(row.LandedMicros)
	if err != nil {
		return EVMCashReturn{}, errs.Wrap(err, errs.CodeInternal, returnEVMCashOp)
	}
	if code, reason := cashRefusal(row, network); reason != "" {
		return EVMCashReturn{}, refuse(code, row, reason)
	}
	if landed.IsZero() {
		return EVMCashReturn{}, refuse(errs.CodeSwapNotStuck, row, "arrival_not_observed")
	}
	return EVMCashReturn{
		Requested: requested, Holder: ids.SwapIDFrom(row.ID), CabalID: ids.CabalIDFrom(row.CabalID),
		Network: network, Landed: landed,
	}, nil
}

func cashRefusal(row sqlc.ReturnableCashRow, network chain.Network) (errs.Code, string) {
	status := domain.Status(row.Status)
	switch {
	case row.BridgeOut != string(domain.StepSucceeded) || !network.IsEVM():
		return errs.CodeSwapNotStuck, "no_landed_bridge_out"
	case row.Returning:
		return errs.CodeStrandedCashTaken, "bridge_back_exists"
	case row.ParkedAt.Valid && status != domain.StatusFailed:
		return errs.CodeSwapNotStuck, "parked_use_unpark_or_fail_parked"
	case status == domain.StatusConfirmed || row.SwapStep == string(domain.StepSucceeded):
		return errs.CodeStrandedCashTaken, "cash_spent"
	case status != domain.StatusFailed:
		return errs.CodeSwapNotStuck, "not_failed"
	case row.SwapStep != "" && row.SwapStep != string(domain.StepFailed):
		return errs.CodeStrandedCashTaken, "cash_spent"
	}
	return "", ""
}

func refuse(code errs.Code, row sqlc.ReturnableCashRow, reason string) error {
	return errs.New(code, returnEVMCashOp, slog.String("swap_id", row.ID.String()), slog.String("reason", reason))
}
