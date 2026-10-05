package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

var _ port.ExternalDeposits = (*Bouncer)(nil)

func (b *Bouncer) Retry(ctx context.Context, id uuid.UUID, operator string) error {
	row, err := b.load(ctx, id)
	if err != nil {
		return err
	}
	if _, err := domain.Next(row.status, domain.BounceRetry); err != nil {
		return err
	}
	opsLog(ctx, id, "retry", operator)
	return b.send(ctx, row)
}

func (b *Bouncer) SetReturnAddress(ctx context.Context, id uuid.UUID, address, operator string) error {
	const op = "funding.Bouncer.SetReturnAddress"
	to, err := chain.ParseAddress(address)
	if err != nil {
		return errs.Wrap(err, errs.CodeInvalidAddress, op)
	}
	err = b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).SetBounceReturnAddress(ctx, sqlc.SetBounceReturnAddressParams{
			ID: id, ReturnAddress: string(to), FromStatuses: statuses(domain.Sources(domain.BounceHold)),
		})
		if err == nil && n == 0 {
			err = refused(op, id)
		}
		return err
	})
	if err == nil {
		opsLog(ctx, id, "set-return-address", operator)
	}
	return err
}

func (b *Bouncer) Hold(ctx context.Context, id uuid.UUID, operator string) error {
	const op = "funding.Bouncer.Hold"
	row, err := b.load(ctx, id)
	if err != nil {
		return err
	}
	err = b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now := b.d.Clock.Now()
		n, err := sqlc.New(tx.Queries()).HoldExternalDeposit(ctx, sqlc.HoldExternalDepositParams{
			ID: id, ResolvedAt: now, FromStatuses: statuses(domain.Sources(domain.BounceHold)),
		})
		if err == nil && n == 0 {
			return refused(op, id)
		}
		b.moved(tx, row, domain.ExternalHeld)
		return errors.Join(err, resolveDepositPauses(ctx, tx, now, b.d.Hints, id))
	})
	if err == nil {
		opsLog(ctx, id, "hold", operator)
	}
	return err
}

func (b *Bouncer) UnresolvedExternalDeposits(ctx context.Context) ([]port.ExternalDeposit, error) {
	rows, err := sqlc.New(b.d.Reads).UnresolvedExternalDeposits(ctx)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, "funding.Bouncer.UnresolvedExternalDeposits")
	}
	out := make([]port.ExternalDeposit, len(rows))
	for i, r := range rows {
		out[i] = port.ExternalDeposit{
			ID: r.ID, CabalID: ids.CabalIDFrom(r.CabalID), Sender: chain.SolanaAddress(r.Sender),
			ReturnAddress: chain.SolanaAddress(r.ReturnAddress), Mint: chain.SolanaAddress(r.Mint), Amount: r.Amount,
			Status: domain.ExternalDepositStatus(r.Status), BounceSignature: chain.Signature(r.BounceSignature),
			BounceAttempts: r.BounceAttempts, DetectedAt: r.DetectedAt,
		}
	}
	return out, nil
}

func statuses(in []domain.ExternalDepositStatus) []string {
	out := make([]string, len(in))
	for i, s := range in {
		out[i] = string(s)
	}
	return out
}

func refused(op string, id uuid.UUID) error {
	return errs.New(errs.CodeVersionConflict, op, slog.String("external_deposit_id", id.String()),
		slog.String("reason", "external deposit is not detected or bounce_failed"))
}

func opsLog(ctx context.Context, id uuid.UUID, action, operator string) {
	observability.Info(ctx, observability.FundingBounceOps, slog.String("external_deposit_id", id.String()),
		slog.String("action", action), slog.String("operator", operator))
}
