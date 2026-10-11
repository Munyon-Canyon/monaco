package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (p *WithdrawalPoller) tickEVM(ctx context.Context, created []sqlc.ListStaleCreatedEVMWithdrawalsRow) (int, error) {
	changed := 0
	failures := make([]error, 0, len(created))
	for _, row := range created {
		moved, err := p.recoverEVM(ctx, row)
		changed += moved
		failures = append(failures, err)
	}
	return changed, errors.Join(failures...)
}

func (p *WithdrawalPoller) recoverEVM(ctx context.Context, row sqlc.ListStaleCreatedEVMWithdrawalsRow) (int, error) {
	request, err := domain.ParseWithdrawalRequest(domain.RawWithdrawal{
		Amount: row.AmountMicros, Kind: string(domain.DestinationEVM),
		Network: row.Network, To: row.EvmAddress,
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeDecodeFailed, "funding.WithdrawalPoller.recoverEVM",
			slog.String("withdrawal_id", row.ID.String()))
	}
	cmd := Withdraw{ID: row.ID, UserID: ids.UserIDFrom(row.UserID), Request: request}
	wallet, err := p.d.Wallets.SigningWallet(ctx, cmd.UserID)
	if err != nil {
		return 0, err
	}
	action, err := p.d.Actions.Transfer(ctx, wallet.ID, evmTransferSpec(request), cmd.ID.String())
	switch {
	case err != nil && transferRefused(err):
		return p.move(ctx, withdrawalMove{
			id: cmd.ID, user: cmd.UserID, amount: request.Amount, from: domain.WithdrawalCreated,
			to: domain.WithdrawalFailed, code: domain.WithdrawalNotSent,
		})
	case err != nil:
		return 0, err
	}
	if err := submitEVM(ctx, p.d.UoW, p.d.Clock.Now(), cmd, action, p.afterCommit); err != nil {
		return 0, err
	}
	return 1, nil
}

func (p *WithdrawalPoller) afterCommit(tx db.Tx, cmd Withdraw, from, to domain.WithdrawalStatus) {
	tx.AfterCommit(func(ctx context.Context) {
		p.d.Hints.PublishHint(ctx, events.UserBalanceChangedHint(cmd.UserID), nil)
		observability.Info(ctx, observability.FundingWithdrawalMoved,
			slog.String("withdrawal_id", cmd.ID.String()), slog.String("user_id", cmd.UserID.String()),
			slog.String("amount_micros", cmd.Request.Amount.String()),
			slog.String("status_before", string(from)), slog.String("status_after", string(to)))
	})
}
