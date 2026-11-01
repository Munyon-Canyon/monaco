package app

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type PayoutTransfers interface {
	Build(context.Context, relayer.TransferSpec) (relayer.SignedTx, error)
	Broadcast(context.Context, relayer.SignedTx) error
}

type PayoutWallets interface {
	TreasuryWallet(context.Context, ids.CabalID) (chain.Wallet, error)
	MemberAddress(context.Context, ids.UserID) (chain.SolanaAddress, error)
}

func (p *CashOutPayouts) sign(ctx context.Context, job payoutJob) error {
	transfers, err := p.d.Transfers()
	if err != nil {
		return err
	}
	from, err := p.d.Wallets.TreasuryWallet(ctx, job.cabal)
	if err != nil {
		return err
	}
	to, err := p.d.Wallets.MemberAddress(ctx, job.user)
	if err != nil {
		return err
	}
	plan, err := p.reserve(ctx, job, to)
	if err != nil {
		return err
	}
	signed, err := transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: from, To: to, Mint: p.d.USDC, Amount: money.NewBaseUnits(job.payout.Uint64(), p.d.USDC.Decimals),
		SkipCreate: !plan.CreateAccount,
	})
	if err != nil && errs.Retryable(errs.CodeOf(err)) {
		return err
	}
	if err != nil {
		return p.fail(ctx, job, 0)
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	return p.store(ctx, job, signed)
}

func payoutSponsorRef(job uuid.UUID, attempt int16) uuid.UUID {
	return uuid.NewSHA1(job, []byte(strconv.Itoa(int(attempt))))
}

func (p *CashOutPayouts) reserve(
	ctx context.Context, job payoutJob, to chain.SolanaAddress,
) (relayer.SponsorPlan, error) {
	var plan relayer.SponsorPlan
	err := p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		plan, err = p.d.Sponsor.Reserve(ctx, tx.Queries(), relayer.SponsorRequest{
			Ref: payoutSponsorRef(job.id, job.latest.Number+1), User: job.user, Destination: to, CanCreate: true,
			Now: p.d.Clock.Now(),
		})
		return err
	})
	return plan, err
}

func (p *CashOutPayouts) release(ctx context.Context, tx db.Tx, job payoutJob) error {
	for _, attempt := range []int16{job.latest.Number, job.latest.Number + 1} {
		if attempt < 1 {
			continue
		}
		if err := p.d.Sponsor.Release(
			ctx,
			tx.Queries(),
			payoutSponsorRef(job.id, attempt),
			p.d.Clock.Now(),
		); err != nil {
			return err
		}
	}
	return nil
}

func (p *CashOutPayouts) store(ctx context.Context, job payoutJob, signed relayer.SignedTx) error {
	return p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		now, err := p.lock(ctx, tx, &job)
		if err != nil || domain.NextPayoutStep(now.status, now.selling, now.latest) != domain.PayoutSign {
			return err
		}
		q := sqlc.New(tx.Queries())
		if now.status == domain.CashOutStarted {
			if err := p.move(ctx, q, now, domain.CashOutStartPaying, ""); err != nil {
				return err
			}
			p.moved(tx, now, domain.CashOutPaying)
		}
		n, err := q.InsertCashOutPayout(ctx, sqlc.InsertCashOutPayoutParams{
			JobID:                now.id,
			Attempt:              now.latest.Number + 1,
			Signature:            string(signed.Signature),
			SignedTx:             signed.Bytes,
			LastValidBlockHeight: strconv.FormatUint(signed.LastValidBlockHeight, 10),
			At:                   p.d.Clock.Now(),
		})
		if err == nil && n == 0 {
			err = errs.New(errs.CodeVersionConflict, "treasury.CashOutPayouts.store",
				slog.String("job_id", now.id.String()))
		}
		return err
	})
}

func (p *CashOutPayouts) send(ctx context.Context, job payoutJob) error {
	p.broadcast(ctx, job)
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	return p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).MoveCashOutPayout(ctx, sqlc.MoveCashOutPayoutParams{
			JobID: job.id, Attempt: job.latest.Number, ToStatus: string(domain.PayoutBroadcast),
			FromStatuses: []string{string(domain.PayoutSigned)},
		})
		return err
	})
}

func (p *CashOutPayouts) broadcast(ctx context.Context, job payoutJob) {
	transfers, err := p.d.Transfers()
	if err == nil {
		err = transfers.Broadcast(ctx, job.signed)
	}
	if err != nil {
		observability.Degraded(ctx, observability.TreasuryCashOutBroadcastFailed,
			slog.String("job_id", job.id.String()), slog.String("code", string(errs.CodeOf(err))),
			slog.String("rpc_message", rpcMessage(err)))
	}
}

func rpcMessage(err error) string {
	for _, a := range errs.Detail(err) {
		if a.Key == "rpc_message" {
			return a.Value.String()
		}
	}
	return ""
}
