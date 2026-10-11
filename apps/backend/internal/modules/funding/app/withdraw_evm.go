package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func (h *WithdrawHandler) handleEVM(ctx context.Context, cmd Withdraw, wallet chain.Wallet) (WithdrawResult, error) {
	if err := h.create(ctx, cmd); err != nil {
		return WithdrawResult{}, err
	}
	action, err := h.d.Actions.Transfer(ctx, wallet.ID, evmTransferSpec(cmd.Request), cmd.ID.String())
	if err != nil {
		if transferRefused(err) {
			return WithdrawResult{}, h.failUnsent(ctx, cmd, err)
		}
		return WithdrawResult{}, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterCreate)
	err = submitEVM(ctx, h.d.UoW, h.d.Clock.Now(), cmd, action, h.afterCommit)
	return WithdrawResult{ID: cmd.ID, Status: domain.WithdrawalSubmitted}, err
}

func evmTransferSpec(r domain.WithdrawalRequest) multichain.TransferSpec {
	return multichain.TransferSpec{
		From: chain.NetworkSolana, To: r.Network, Destination: r.EVM.String(), AmountMicros: r.Amount,
	}
}

func transferRefused(err error) bool {
	kind := errs.KindOf(errs.CodeOf(err))
	return kind == errs.KindInvalid || kind == errs.KindNotFound
}

type afterCommitFunc func(tx db.Tx, cmd Withdraw, from, to domain.WithdrawalStatus)

func submitEVM(
	ctx context.Context, uow *db.UnitOfWork, now time.Time, cmd Withdraw, action multichain.ActionID,
	afterCommit afterCommitFunc,
) error {
	return uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		moved, err := sqlc.New(tx.Queries()).SubmitEVMWithdrawal(ctx, sqlc.SubmitEVMWithdrawalParams{
			ID: cmd.ID, PrivyActionID: string(action), SubmittedAt: now,
		})
		if err != nil || moved == 0 {
			return err
		}
		afterCommit(tx, cmd, domain.WithdrawalCreated, domain.WithdrawalSubmitted)
		return tx.Events.Append(ctx, events.WithdrawalSubmitted{
			V: 1, WithdrawalID: cmd.ID, UserID: cmd.UserID.UUID(), AmountMicros: cmd.Request.Amount,
			ToAddress: chain.SolanaAddress(cmd.Request.Address()),
		})
	})
}
