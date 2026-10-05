package app

import (
	"context"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	BounceWait = 60 * time.Second
	BouncePoll = 2 * time.Second
)

type BounceChain interface {
	SignatureStatuses
	BlockhashValid(ctx context.Context, hash string) (bool, error)
	MintConfig(ctx context.Context, mint chain.SolanaAddress) (solana.MintConfig, error)
	Accounts(ctx context.Context, addrs []chain.SolanaAddress, minContextSlot uint64) (
		uint64, []solana.TokenAccountState, error)
}

type BounceTreasuries interface {
	TreasuryWallet(ctx context.Context, id ids.CabalID) (cabalport.TreasuryWallet, error)
}

type BounceDeps struct {
	UoW        *db.UnitOfWork
	Reads      sqlc.DBTX
	Clock      clock.Clock
	Hints      HintPublisher
	Chain      BounceChain
	Treasuries BounceTreasuries
	Transfers  func() (Transfers, error)
	Failed     metric.Int64Counter
}

type Bouncer struct{ d BounceDeps }

func NewBouncer(d BounceDeps) *Bouncer { return &Bouncer{d: d} }

type bounceRow struct {
	id        uuid.UUID
	cabal     ids.CabalID
	recipient chain.SolanaAddress
	mint      chain.SolanaAddress
	amount    uint64
	status    domain.ExternalDepositStatus
	signed    relayer.SignedTx
	attempts  int32
}

func (b *Bouncer) Start(ctx context.Context, id uuid.UUID) error {
	row, err := b.load(ctx, id)
	if err != nil || row.status != domain.ExternalDetected {
		return err
	}
	signed, err := b.sign(ctx, row)
	if err != nil || signed == nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.AfterSign)
	started, err := b.store(ctx, row, *signed)
	if err != nil || !started {
		return err
	}
	row.status, row.signed = domain.ExternalBouncing, *signed
	if err := b.broadcast(ctx, row); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.AfterBroadcast)
	return b.await(ctx, row)
}

func (b *Bouncer) sign(ctx context.Context, row bounceRow) (*relayer.SignedTx, error) {
	const op = "funding.Bouncer.sign"
	cfg, err := b.d.Chain.MintConfig(ctx, row.mint)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	ata, err := chain.AssociatedTokenAccount(row.recipient, row.mint, cfg.TokenProgram)
	if err != nil {
		return nil, b.fail(ctx, row, "recipient_address_invalid")
	}
	_, accounts, err := b.d.Chain.Accounts(ctx, []chain.SolanaAddress{ata}, 0)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if len(accounts) != 1 || !accounts[0].Exists {
		return nil, b.fail(ctx, row, "recipient_token_account_closed")
	}
	treasury, err := b.d.Treasuries.TreasuryWallet(ctx, row.cabal)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), op)
	}
	transfers, err := b.d.Transfers()
	if err != nil {
		return nil, err
	}
	signed, err := transfers.Build(ctx, relayer.TransferSpec{
		FromWallet: chain.Wallet{ID: treasury.PrivyWalletID, Address: treasury.Address},
		To:         row.recipient, Mint: cfg.Mint, Amount: money.NewBaseUnits(row.amount, cfg.Mint.Decimals),
	})
	if err != nil && errs.Retryable(errs.CodeOf(err)) {
		return nil, err
	}
	if err != nil {
		return nil, b.fail(ctx, row, string(errs.CodeOf(err)))
	}
	return &signed, nil
}

func (b *Bouncer) store(ctx context.Context, row bounceRow, signed relayer.SignedTx) (bool, error) {
	started := false
	err := b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).StartBounce(ctx, sqlc.StartBounceParams{
			ID: row.id, FromStatus: string(row.status), BounceSignature: string(signed.Signature),
			BounceSignedTx: signed.Bytes,
		})
		if err != nil || n == 0 {
			return err
		}
		started = true
		b.moved(tx, row, domain.ExternalBouncing)
		return nil
	})
	return started, err
}

func (b *Bouncer) broadcast(ctx context.Context, row bounceRow) error {
	transfers, err := b.d.Transfers()
	if err == nil {
		err = transfers.Broadcast(ctx, row.signed)
	}
	return err
}

func (b *Bouncer) await(ctx context.Context, row bounceRow) error {
	deadline := b.d.Clock.Now().Add(BounceWait)
	for {
		done, err := b.Check(ctx, row.id)
		if err != nil || done || !b.d.Clock.Now().Before(deadline) {
			return err
		}
		select {
		case <-ctx.Done():
			return errs.Wrap(ctx.Err(), errs.CodeUpstreamUnavailable, "funding.Bouncer.await")
		case <-b.d.Clock.After(BouncePoll):
		}
	}
}

func (b *Bouncer) Check(ctx context.Context, id uuid.UUID) (bool, error) {
	row, err := b.load(ctx, id)
	if err != nil || row.status != domain.ExternalBouncing {
		return true, err
	}
	statuses, err := b.d.Chain.SignatureStatuses(ctx, []chain.Signature{row.signed.Signature})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), "funding.Bouncer.Check")
	}
	if len(statuses) != 1 || statuses[0].State != solana.StateFinalized {
		return false, nil
	}
	if statuses[0].Failed {
		return true, b.fail(ctx, row, "transaction_failed")
	}
	return true, b.resolve(ctx, row)
}

func (b *Bouncer) resolve(ctx context.Context, row bounceRow) error {
	return b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		now := b.d.Clock.Now()
		n, err := q.ReturnBounce(ctx, sqlc.ReturnBounceParams{
			ID: row.id, BounceSignature: string(row.signed.Signature), ResolvedAt: now,
		})
		if err != nil || n == 0 {
			return err
		}
		b.moved(tx, row, domain.ExternalReturned)
		return errors.Join(resolveDepositPauses(ctx, tx, now, b.d.Hints, row.id),
			tx.Events.Append(ctx, events.CabalExternalDepositBounced{
				V: 1, ExternalDepositID: row.id, CabalID: row.cabal.UUID(), BounceSignature: row.signed.Signature,
				Recipient: row.recipient, Mint: row.mint, Amount: row.amount,
			}))
	})
}

func resolveDepositPauses(ctx context.Context, tx db.Tx, at time.Time, hints HintPublisher, id uuid.UUID) error {
	pauses, err := sqlc.New(tx.Queries()).OpenExternalDepositPauses(ctx, id)
	for _, pause := range pauses {
		err = errors.Join(err, ResolvePause(ctx, tx, at, hints, pause))
	}
	return err
}

func (b *Bouncer) fail(ctx context.Context, row bounceRow, reason string) error {
	moved := false
	err := b.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		n, err := sqlc.New(tx.Queries()).FailBounce(ctx, sqlc.FailBounceParams{
			ID: row.id, FromStatus: string(row.status),
		})
		if err != nil || n == 0 {
			return err
		}
		moved = true
		b.moved(tx, row, domain.ExternalBounceFailed)
		return nil
	})
	if err != nil || !moved {
		return err
	}
	observability.Degraded(ctx, observability.FundingBounceFailed,
		slog.String("external_deposit_id", row.id.String()), slog.String("cabal_id", row.cabal.String()),
		slog.String("reason", reason))
	b.d.Failed.Add(ctx, 1)
	return nil
}

func (b *Bouncer) moved(tx db.Tx, row bounceRow, to domain.ExternalDepositStatus) {
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.FundingBounceMoved,
			slog.String("external_deposit_id", row.id.String()), slog.String("cabal_id", row.cabal.String()),
			slog.String("status_before", string(row.status)), slog.String("status_after", string(to)))
	})
}

func (b *Bouncer) load(ctx context.Context, id uuid.UUID) (bounceRow, error) {
	const op = "funding.Bouncer.load"
	r, err := sqlc.New(b.d.Reads).ExternalDepositForBounce(ctx, id)
	if err != nil {
		return bounceRow{}, errs.Wrap(err, errs.CodeInternal, op, slog.String("external_deposit_id", id.String()))
	}
	amount, err := strconv.ParseUint(r.Amount, 10, 64)
	if err != nil {
		return bounceRow{}, errs.Wrap(err, errs.CodeDecodeFailed, op, slog.String("external_deposit_id", id.String()))
	}
	recipient := chain.SolanaAddress(r.ReturnAddress)
	if recipient == "" {
		recipient = chain.SolanaAddress(r.Sender)
	}
	return bounceRow{
		id: r.ID, cabal: ids.CabalIDFrom(r.CabalID), recipient: recipient, mint: chain.SolanaAddress(r.Mint),
		amount: amount, status: domain.ExternalDepositStatus(r.Status),
		signed:   relayer.SignedTx{Bytes: r.BounceSignedTx, Signature: chain.Signature(r.BounceSignature)},
		attempts: r.BounceAttempts,
	}, nil
}
