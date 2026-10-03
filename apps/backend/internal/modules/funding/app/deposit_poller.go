package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"time"

	"golang.org/x/time/rate"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const DepositPollInterval = 30 * time.Second

var errRateBudgetSpent = errs.New(errs.CodeInternal, "funding.DepositPoller.rateBudgetSpent")

type DepositRPC interface {
	SignaturesFor(
		context.Context, chain.SolanaAddress, chain.Signature, chain.Signature, int,
	) ([]solana.SignatureInfo, error)
	InboundTransfersForMint(
		context.Context, chain.Signature, chain.SolanaAddress, chain.SolanaAddress,
	) ([]solana.Transfer, error)
}

type DepositPoller struct {
	reads   sqlc.DBTX
	uow     *db.UnitOfWork
	ids     ids.Generator
	clock   clock.Clock
	wallets port.WalletReader
	rpc     DepositRPC
	usdc    chain.SolanaAddress
	period  time.Duration
	hints   HintPublisher
	limit   *rate.Limiter
}

func NewDepositPoller(
	reads sqlc.DBTX, uow *db.UnitOfWork, g ids.Generator, c clock.Clock, wallets port.WalletReader,
	rpc DepositRPC, usdc chain.SolanaAddress, period time.Duration, rpcRate int32, hints HintPublisher,
) *DepositPoller {
	if rpcRate <= 0 {
		rpcRate = 20
	}
	return &DepositPoller{
		reads: reads, uow: uow, ids: g, clock: c, wallets: wallets, rpc: rpc, usdc: usdc,
		period: period, hints: hints, limit: rate.NewLimiter(rate.Limit(rpcRate), int(rpcRate)),
	}
}

func (*DepositPoller) Name() string { return "funding.deposits" }

func (p *DepositPoller) Interval() time.Duration { return p.period }

func (p *DepositPoller) Tick(ctx context.Context) (poller.Report, error) {
	var report poller.Report
	wallets, err := p.memberWallets(ctx)
	if err != nil {
		return report, err
	}
	changed, deferred, tickErr := p.scanWallets(ctx, wallets)
	report.Scanned = len(wallets) - deferred
	report.Changed = changed
	report.Attrs = []slog.Attr{slog.Int("deferred", deferred)}
	return report, tickErr
}

func (p *DepositPoller) scanWallets(ctx context.Context, wallets []port.MemberWallet) (int, int, error) {
	out, errc := concurrency.Pool(ctx, 16, concurrency.Feed(ctx, wallets), p.scan)
	var changed, deferred int
	var scanErr error
	for out != nil || errc != nil {
		select {
		case delta, ok := <-out:
			if !ok {
				out = nil
				continue
			}
			changed += delta
		case err, ok := <-errc:
			if !ok {
				errc = nil
				continue
			}
			if errors.Is(err, errRateBudgetSpent) {
				deferred++
				continue
			}
			scanErr = errors.Join(scanErr, err)
		}
	}
	if err := context.Cause(ctx); err != nil {
		return changed, deferred, errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.scanWallets")
	}
	return changed, deferred, scanErr
}

func (p *DepositPoller) memberWallets(ctx context.Context) ([]port.MemberWallet, error) {
	var wallets []port.MemberWallet
	var after ids.UserID
	for {
		page, err := p.wallets.MemberWallets(ctx, after, port.MaxWalletPage)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.memberWallets")
		}
		wallets = append(wallets, page...)
		if len(page) < port.MaxWalletPage {
			seen := make(map[chain.SolanaAddress]time.Time, len(wallets))
			for _, wallet := range wallets {
				cursor, err := sqlc.New(p.reads).DepositCursor(ctx, string(wallet.Address))
				if err != nil {
					return nil, errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.memberWallets")
				}
				seen[wallet.Address] = cursor.ScannedAt
			}
			slices.SortFunc(wallets, func(a, b port.MemberWallet) int {
				return seen[a.Address].Compare(seen[b.Address])
			})
			return wallets, nil
		}
		after = page[len(page)-1].UserID
	}
}

func (p *DepositPoller) scan(ctx context.Context, wallet port.MemberWallet) (int, error) {
	ata, err := chain.AssociatedTokenAccount(wallet.Address, p.usdc, chain.SPLProgram)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInvalidAddress, "funding.DepositPoller.scan")
	}
	cursor, err := sqlc.New(p.reads).DepositCursor(ctx, string(wallet.Address))
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.scan")
	}
	sigs, err := p.signaturesSince(ctx, ata, chain.Signature(cursor.LastSignature))
	if err != nil {
		return 0, err
	}
	if len(sigs) == 0 {
		return 0, p.touch(ctx, wallet.Address)
	}
	slices.Reverse(sigs)
	changed := 0
	for _, sig := range sigs {
		added, err := p.scanSignature(ctx, wallet, sig)
		if err != nil {
			return changed, err
		}
		changed += added
	}
	return changed, nil
}

func (p *DepositPoller) touch(ctx context.Context, address chain.SolanaAddress) error {
	return p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).TouchDepositCursor(ctx, sqlc.TouchDepositCursorParams{
			WalletAddress: string(address), ScannedAt: p.clock.Now(),
		})
	})
}

func (p *DepositPoller) signaturesSince(
	ctx context.Context, address chain.SolanaAddress, until chain.Signature,
) ([]solana.SignatureInfo, error) {
	const pageSize = 1000
	var all []solana.SignatureInfo
	var before chain.Signature
	for {
		if err := p.waitRPC(ctx); err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.signatures")
		}
		page, err := p.rpc.SignaturesFor(ctx, address, before, until, pageSize)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeRPCUnavailable, "funding.DepositPoller.signatures")
		}
		all = append(all, page...)
		if len(page) < pageSize {
			return all, nil
		}
		before = page[len(page)-1].Signature
	}
}

func (p *DepositPoller) waitRPC(ctx context.Context) error {
	err := p.limit.Wait(ctx)
	if err == nil {
		return nil
	}
	if ctx.Err() == nil {
		return errRateBudgetSpent
	}
	return errs.Wrap(err, errs.CodeInternal, "funding.DepositPoller.waitRPC")
}

func (p *DepositPoller) scanSignature(
	ctx context.Context,
	wallet port.MemberWallet,
	sig solana.SignatureInfo,
) (int, error) {
	if sig.Failed {
		return 0, p.advance(ctx, wallet.Address, sig.Signature, sig.Slot)
	}
	if err := p.waitRPC(ctx); err != nil {
		return 0, fmt.Errorf("funding.DepositPoller.scanSignature: %w", err)
	}
	transfers, err := p.rpc.InboundTransfersForMint(ctx, sig.Signature, wallet.Address, p.usdc)
	if err != nil {
		return 0, err
	}
	amount, err := p.amountFor(transfers)
	if err != nil {
		return 0, err
	}
	if amount.IsZero() {
		return 0, p.advance(ctx, wallet.Address, sig.Signature, sig.Slot)
	}
	credited, err := p.credit(ctx, wallet, sig, amount)
	if err != nil {
		return 0, err
	}
	if credited {
		return 1, nil
	}
	return 0, nil
}

func (p *DepositPoller) amountFor(transfers []solana.Transfer) (money.Micros, error) {
	amount := money.Micros{}
	for _, transfer := range transfers {
		if transfer.Mint.Address != p.usdc || transfer.Net.Decimals() != 6 || transfer.Net.IsZero() {
			continue
		}
		micros := money.MicrosFromUint64(transfer.Net.Uint64())
		next, err := amount.Add(micros)
		if err != nil {
			return money.Micros{}, err
		}
		amount = next
	}
	return amount, nil
}

func (p *DepositPoller) credit(
	ctx context.Context,
	wallet port.MemberWallet,
	sig solana.SignatureInfo,
	amount money.Micros,
) (bool, error) {
	if sig.Slot > math.MaxInt64 {
		return false, errs.New(errs.CodeInternal, "funding.DepositPoller.credit")
	}
	credited, err := NewCreditDepositHandler(p.uow, p.hints).Handle(ctx, CreditDeposit{
		ID:              p.ids.NewV7(),
		UserID:          wallet.UserID,
		WalletAddress:   wallet.Address,
		TxSignature:     sig.Signature,
		Amount:          amount,
		Slot:            int64(sig.Slot),
		BlockTime:       sig.BlockTime,
		CreditedAt:      p.clock.Now(),
		CursorSignature: sig.Signature,
	})
	if err != nil {
		return false, errs.Wrap(err, errs.CodeOf(err), "funding.DepositPoller.credit")
	}
	return credited, nil
}

func (p *DepositPoller) advance(
	ctx context.Context,
	address chain.SolanaAddress,
	sig chain.Signature,
	slot uint64,
) error {
	if slot > math.MaxInt64 {
		return errs.New(errs.CodeInternal, "funding.DepositPoller.advance")
	}
	slotInt := int64(slot)
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).AdvanceDepositCursor(ctx, sqlc.AdvanceDepositCursorParams{
			WalletAddress: string(address), LastSignature: string(sig), CursorSlot: slotInt,
			ScannedAt: p.clock.Now(),
		})
	})
	if err != nil {
		return errs.Wrap(err, errs.CodeOf(err), "funding.DepositPoller.advance")
	}
	return nil
}
