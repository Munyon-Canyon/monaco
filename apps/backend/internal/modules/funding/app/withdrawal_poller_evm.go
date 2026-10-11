package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (p *WithdrawalPoller) tickEVM(
	ctx context.Context, created []sqlc.ListStaleCreatedEVMWithdrawalsRow,
	submitted []sqlc.ListSubmittedEVMWithdrawalsRow,
) (int, error) {
	changed := 0
	failures := make([]error, 0, len(created)+len(submitted))
	for _, row := range created {
		moved, err := p.recoverEVM(ctx, row)
		changed += moved
		failures = append(failures, err)
	}
	for _, row := range submitted {
		moved, err := p.resolveEVM(ctx, row)
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
	if err == nil {
		err = p.d.TopUp.TopUpSOL(ctx, wallet.Address)
	}
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

type evmOutcome struct {
	move         withdrawalMove
	record       chain.Signature
	needsSupport bool
}

func (p *WithdrawalPoller) resolveEVM(ctx context.Context, row sqlc.ListSubmittedEVMWithdrawalsRow) (int, error) {
	wallet, err := p.d.Wallets.SigningWallet(ctx, ids.UserIDFrom(row.UserID))
	if err != nil {
		return 0, err
	}
	status, err := p.d.Actions.Action(ctx, wallet.ID, multichain.ActionID(row.PrivyActionID))
	if err != nil {
		return 0, err
	}
	out, err := evmOutcomeOf(row, status)
	if err != nil {
		return 0, err
	}
	if out.needsSupport {
		p.flagNeedsSupport(ctx, row)
	}
	return p.settleEVM(ctx, row, out)
}

func (p *WithdrawalPoller) flagNeedsSupport(ctx context.Context, row sqlc.ListSubmittedEVMWithdrawalsRow) {
	if _, seen := p.flagged.LoadOrStore(row.ID, struct{}{}); seen {
		return
	}
	observability.Degraded(ctx, observability.FundingWithdrawalNeedsSupport,
		slog.String("withdrawal_id", row.ID.String()), slog.String("privy_action_id", row.PrivyActionID))
}

func evmOutcomeOf(row sqlc.ListSubmittedEVMWithdrawalsRow, status multichain.ActionStatus) (evmOutcome, error) {
	amount, amountErr := money.ParseMicros(row.AmountMicros)
	out, err := evmTransition(row, status, amount)
	return out, errors.Join(amountErr, err)
}

func evmTransition(
	row sqlc.ListSubmittedEVMWithdrawalsRow, status multichain.ActionStatus, amount money.Micros,
) (evmOutcome, error) {
	source, sent := solanaStep(status)
	out := evmOutcome{move: withdrawalMove{
		id: row.ID, user: ids.UserIDFrom(row.UserID), amount: amount, from: domain.WithdrawalSubmitted,
	}}
	if sent && row.TxSignature == "" {
		out.record = source
	}
	switch status.State {
	case multichain.ActionPending:
		return out, nil
	case multichain.ActionSucceeded:
		if !sent {
			return out, evmDecodeFailed(row, "succeeded without a confirmed solana step")
		}
		out.move.to = domain.WithdrawalConfirmed
		out.move.event = events.WithdrawalConfirmed{
			V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount,
			ToAddress: chain.SolanaAddress(row.EvmAddress), TxSignature: source,
		}
		return out, nil
	case multichain.ActionFailed:
		if anyStepBroadcast(status) {
			out.needsSupport = true
			return out, nil
		}
		out.move.to, out.move.code = domain.WithdrawalFailed, domain.WithdrawalTransferFailed
	case multichain.ActionRejected:
		out.move.to, out.move.code = domain.WithdrawalFailed, domain.WithdrawalTransferRejected
	default:
		return out, evmDecodeFailed(row, "unknown action state "+string(status.State))
	}
	out.move.event = events.WithdrawalFailed{
		V: 1, WithdrawalID: row.ID, UserID: row.UserID, AmountMicros: amount, Code: out.move.code,
	}
	return out, nil
}

func evmDecodeFailed(row sqlc.ListSubmittedEVMWithdrawalsRow, reason string) error {
	return errs.New(errs.CodeDecodeFailed, "funding.WithdrawalPoller.resolveEVM",
		slog.String("withdrawal_id", row.ID.String()), slog.String("reason", reason))
}

func solanaStep(status multichain.ActionStatus) (chain.Signature, bool) {
	for _, step := range status.Steps {
		if step.Network == chain.NetworkSolana && step.State == multichain.StepConfirmed && step.TxHash != "" {
			return chain.Signature(step.TxHash), true
		}
	}
	return "", false
}

func anyStepBroadcast(status multichain.ActionStatus) bool {
	for _, step := range status.Steps {
		if step.State == multichain.StepConfirmed || step.TxHash != "" {
			return true
		}
	}
	return false
}

func (p *WithdrawalPoller) settleEVM(
	ctx context.Context, row sqlc.ListSubmittedEVMWithdrawalsRow, out evmOutcome,
) (int, error) {
	if out.record == "" && out.move.to == "" {
		return 0, nil
	}
	moved := 0
	err := p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).SetWithdrawalSourceSignature(ctx, sqlc.SetWithdrawalSourceSignatureParams{
			ID: row.ID, TxSignature: string(out.record),
		})
		if err != nil || out.move.to == "" {
			return err
		}
		moved, err = p.apply(ctx, tx, out.move)
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.WithdrawalPoller.settleEVM")
	}
	return moved, nil
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
