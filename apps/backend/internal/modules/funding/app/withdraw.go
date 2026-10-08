package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type SigningWallets interface {
	SigningWallet(context.Context, ids.UserID) (chain.Wallet, error)
}

type Transfers interface {
	Build(context.Context, relayer.TransferSpec) (relayer.SignedTx, error)
	Broadcast(context.Context, relayer.SignedTx) error
}

type Withdraw struct {
	ID      uuid.UUID
	UserID  ids.UserID
	Request domain.WithdrawalRequest
}

type WithdrawResult struct {
	ID          uuid.UUID
	Status      domain.WithdrawalStatus
	TxSignature chain.Signature
}

type WithdrawDeps struct {
	UoW       *db.UnitOfWork
	Balances  port.Balances
	Wallets   SigningWallets
	Transfers func() (Transfers, error)
	Hints     HintPublisher
	Clock     clock.Clock
	USDC      chain.Mint
}

type WithdrawHandler struct{ d WithdrawDeps }

func NewWithdrawHandler(d WithdrawDeps) *WithdrawHandler { return &WithdrawHandler{d: d} }

func (h *WithdrawHandler) Handle(ctx context.Context, cmd Withdraw) (WithdrawResult, error) {
	const op = "funding.Withdraw.Handle"
	wallet, err := h.d.Wallets.SigningWallet(ctx, cmd.UserID)
	if err != nil {
		return WithdrawResult{}, err
	}
	if wallet.Address == cmd.Request.To {
		return WithdrawResult{}, errs.New(errs.CodeWithdrawToOwnWallet, op)
	}
	transfers, err := h.d.Transfers()
	if err != nil {
		return WithdrawResult{}, err
	}
	if err := h.create(ctx, cmd); err != nil {
		return WithdrawResult{}, err
	}
	signed, err := transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: wallet, To: cmd.Request.To, Mint: h.d.USDC,
		Amount: money.NewBaseUnits(cmd.Request.Amount.Uint64(), h.d.USDC.Decimals),
	})
	if err != nil {
		return WithdrawResult{}, h.failUnsent(ctx, cmd, err)
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	if err := h.submit(ctx, cmd, signed); err != nil {
		return WithdrawResult{}, err
	}
	if err := transfers.Broadcast(ctx, signed); err != nil {
		observability.Degraded(ctx, observability.FundingWithdrawalBroadcastFailed,
			slog.String("withdrawal_id", cmd.ID.String()), slog.String("code", string(errs.CodeOf(err))))
	}
	return WithdrawResult{ID: cmd.ID, Status: domain.WithdrawalSubmitted, TxSignature: signed.Signature}, nil
}

func (h *WithdrawHandler) create(ctx context.Context, cmd Withdraw) error {
	const op = "funding.Withdraw.create"
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		if err := q.LockWalletOutflow(ctx, cmd.UserID.UUID()); err != nil {
			return err
		}
		balance, err := h.d.Balances.Available(ctx, cmd.UserID)
		if err != nil {
			return err
		}
		if balance.AvailableMicros.Cmp(cmd.Request.Amount) < 0 {
			return errs.New(errs.CodeInsufficientFunds, op, slog.String("available_micros",
				balance.AvailableMicros.String()), slog.String("amount_micros", cmd.Request.Amount.String()))
		}
		if err := q.InsertWithdrawal(ctx, sqlc.InsertWithdrawalParams{
			ID: cmd.ID, UserID: cmd.UserID.UUID(), ToAddress: string(cmd.Request.To),
			CreatedAt: h.d.Clock.Now(), AmountMicros: cmd.Request.Amount.String(),
		}); err != nil {
			return err
		}
		h.afterCommit(tx, cmd, "", domain.WithdrawalCreated)
		return nil
	})
}

func (h *WithdrawHandler) failUnsent(ctx context.Context, cmd Withdraw, cause error) error {
	code := errs.CodeOf(cause)
	err := h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).FailWithdrawal(ctx, sqlc.FailWithdrawalParams{
			ID: cmd.ID, FailCode: string(code), CompletedAt: h.d.Clock.Now(),
		})
		if err != nil || moved == 0 {
			return err
		}
		h.afterCommit(tx, cmd, domain.WithdrawalCreated, domain.WithdrawalFailed)
		return nil
	})
	if err != nil {
		return errors.Join(cause, err)
	}
	return cause
}

func (h *WithdrawHandler) submit(ctx context.Context, cmd Withdraw, signed relayer.SignedTx) error {
	const op = "funding.Withdraw.submit"
	return h.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).SubmitWithdrawal(ctx, sqlc.SubmitWithdrawalParams{
			ID: cmd.ID, SignedTx: signed.Bytes, TxSignature: string(signed.Signature),
			LastValidBlockHeight: strconv.FormatUint(signed.LastValidBlockHeight, 10), SubmittedAt: h.d.Clock.Now(),
		})
		if err != nil {
			return err
		}
		if moved == 0 {
			return errs.New(errs.CodeInternal, op, slog.String("withdrawal_id", cmd.ID.String()),
				slog.String("reason", "withdrawal left created before submit"))
		}
		if err := tx.Events.Append(ctx, events.WithdrawalSubmitted{
			V: 1, WithdrawalID: cmd.ID, UserID: cmd.UserID.UUID(), AmountMicros: cmd.Request.Amount,
			ToAddress: cmd.Request.To, TxSignature: signed.Signature,
		}); err != nil {
			return err
		}
		h.afterCommit(tx, cmd, domain.WithdrawalCreated, domain.WithdrawalSubmitted)
		return nil
	})
}

func (h *WithdrawHandler) afterCommit(tx db.Tx, cmd Withdraw, from, to domain.WithdrawalStatus) {
	tx.AfterCommit(func(ctx context.Context) {
		h.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(cmd.UserID), nil)
		observability.Info(ctx, observability.FundingWithdrawalMoved,
			slog.String("withdrawal_id", cmd.ID.String()), slog.String("user_id", cmd.UserID.String()),
			slog.String("amount_micros", cmd.Request.Amount.String()),
			slog.String("status_before", string(from)), slog.String("status_after", string(to)))
	})
}
