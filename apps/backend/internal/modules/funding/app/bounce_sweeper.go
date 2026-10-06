package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	BounceSweepInterval = 30 * time.Second
	BounceSweepAge      = 2 * time.Minute
	BounceMaxAttempts   = 3
	bounceSweepBatch    = 50
)

type BounceSweepTiming struct {
	Interval time.Duration
	Age      time.Duration
}

func DefaultBounceSweepTiming() BounceSweepTiming {
	return BounceSweepTiming{Interval: BounceSweepInterval, Age: BounceSweepAge}
}

type BounceSweeper struct {
	b      *Bouncer
	timing BounceSweepTiming
}

func NewBounceSweeper(b *Bouncer, timing BounceSweepTiming) *BounceSweeper {
	return &BounceSweeper{b: b, timing: timing}
}

func (*BounceSweeper) Name() string { return "funding.bounce-sweeper" }

func (s *BounceSweeper) Interval() time.Duration { return s.timing.Interval }

func (s *BounceSweeper) Tick(ctx context.Context) (poller.Report, error) {
	due, err := sqlc.New(s.b.d.Reads).ListStaleBounces(ctx, sqlc.ListStaleBouncesParams{
		OlderThan: s.b.d.Clock.Now().Add(-s.timing.Age), MaxRows: bounceSweepBatch,
	})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "funding.BounceSweeper.list")
	}
	report := poller.Report{Scanned: len(due)}
	failures := make([]error, 0, len(due))
	for _, id := range due {
		changed, err := s.b.Sweep(ctx, id)
		if changed {
			report.Changed++
		}
		failures = append(failures, err)
	}
	return report, errors.Join(failures...)
}

func (b *Bouncer) Sweep(ctx context.Context, id uuid.UUID) (bool, error) {
	row, err := b.load(ctx, id)
	if err != nil || row.status != domain.ExternalBouncing {
		return false, err
	}
	statuses, err := b.d.Chain.SignatureStatuses(ctx, []chain.Signature{row.signed.Signature})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), "funding.Bouncer.Sweep")
	}
	status := solana.Status{State: solana.StateProcessing}
	if len(statuses) == 1 {
		status = statuses[0]
	}
	switch {
	case status.State == solana.StateFinalized && status.Failed:
		return true, b.fail(ctx, row, "transaction_failed")
	case status.State == solana.StateFinalized:
		return true, b.resolve(ctx, row)
	case status.State == solana.StateNotFound:
		return b.lapsed(ctx, row)
	default:
		return false, nil
	}
}

func (b *Bouncer) lapsed(ctx context.Context, row bounceRow) (bool, error) {
	const op = "funding.Bouncer.lapsed"
	hash, err := chain.RecentBlockhash(row.signed.Bytes)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	valid, err := b.d.Chain.BlockhashValid(ctx, hash)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if valid {
		return false, b.broadcast(ctx, row)
	}
	if row.attempts >= BounceMaxAttempts {
		return true, b.fail(ctx, row, "blockhash_expired")
	}
	return b.resign(ctx, row)
}

func (b *Bouncer) resign(ctx context.Context, row bounceRow) (bool, error) {
	signed, err := b.sign(ctx, row)
	if err != nil {
		return false, err
	}
	if signed == nil {
		return true, nil
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	stored, err := b.replace(ctx, row, *signed)
	if err != nil || !stored {
		return false, err
	}
	row.signed = *signed
	if err := b.broadcast(ctx, row); err != nil {
		return true, err
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	return true, nil
}

func (b *Bouncer) replace(ctx context.Context, row bounceRow, signed relayer.SignedTx) (bool, error) {
	stored := false
	err := b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).ResignBounce(ctx, sqlc.ResignBounceParams{
			ID: row.id, OldSignature: string(row.signed.Signature), BounceSignature: string(signed.Signature),
			BounceSignedTx: signed.Bytes,
		})
		stored = n == 1
		return err
	})
	return stored, err
}
