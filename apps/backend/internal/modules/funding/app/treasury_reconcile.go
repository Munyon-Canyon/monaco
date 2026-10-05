package app

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	reconcileFanOut        = 8
	reconcilePageSize      = 50
	reconcileBootstrapPage = 20
	usdcDecimals           = 6
)

type ReconcileChain interface {
	TokenBalance(ctx context.Context, owner chain.SolanaAddress, mint chain.Mint) (money.BaseUnits, error)
	SignaturesFor(
		ctx context.Context,
		addr chain.SolanaAddress,
		in solana.SignaturesOpts,
	) ([]solana.SignatureInfo, error)
}

type LedgerPositions interface {
	Positions(ctx context.Context, cabalID ids.CabalID) ([]treasuryport.Position, error)
}

type TreasuryWallets interface {
	TreasuryWallets(ctx context.Context) ([]cabalport.TreasuryWallet, error)
}

type Detector interface {
	Handle(ctx context.Context, cmd DetectExternalDeposit) (DetectResult, error)
}

type TreasuryReconcileDeps struct {
	UoW        *db.UnitOfWork
	Reads      sqlc.DBTX
	Clock      clock.Clock
	Treasuries TreasuryWallets
	Ledger     LedgerPositions
	Chain      ReconcileChain
	Detect     Detector
	USDC       chain.SolanaAddress
	Interval   time.Duration
}

type TreasuryReconcilePoller struct{ d TreasuryReconcileDeps }

func NewTreasuryReconcilePoller(d TreasuryReconcileDeps) *TreasuryReconcilePoller {
	return &TreasuryReconcilePoller{d: d}
}

func (*TreasuryReconcilePoller) Name() string { return "funding.treasury-reconcile" }

func (p *TreasuryReconcilePoller) Interval() time.Duration { return p.d.Interval }

type reconciled struct {
	surplus  bool
	recorded int
	err      error
}

func (p *TreasuryReconcilePoller) Tick(ctx context.Context) (poller.Report, error) {
	wallets, err := p.d.Treasuries.TreasuryWallets(ctx)
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), "funding.TreasuryReconcile.Tick")
	}
	results, err := concurrency.FanOut(ctx, reconcileFanOut, wallets,
		func(ctx context.Context, w cabalport.TreasuryWallet) (reconciled, error) {
			surplus, recorded, err := p.reconcile(ctx, w)
			return reconciled{surplus: surplus, recorded: recorded, err: err}, nil
		})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "funding.TreasuryReconcile.Tick")
	}
	report := poller.Report{Scanned: len(wallets)}
	var surpluses int
	var tickErr error
	for _, r := range results {
		report.Changed += r.recorded
		if r.surplus {
			surpluses++
		}
		tickErr = errors.Join(tickErr, r.err)
	}
	report.Attrs = []slog.Attr{slog.Int("surpluses", surpluses)}
	return report, tickErr
}

func (p *TreasuryReconcilePoller) reconcile(ctx context.Context, w cabalport.TreasuryWallet) (bool, int, error) {
	const op = "funding.TreasuryReconcile.reconcile"
	surplus, err := p.hasSurplus(ctx, w)
	if err != nil || !surplus {
		return false, 0, err
	}
	cursor, err := sqlc.New(p.d.Reads).WatchCursor(ctx, w.CabalID.UUID())
	if err != nil {
		return true, 0, errs.Wrap(err, errs.CodeInternal, op)
	}
	sigs, err := p.signaturesSince(ctx, w.Address, chain.Signature(cursor))
	if err != nil || len(sigs) == 0 {
		return true, 0, err
	}
	recorded := 0
	for _, s := range slices.Backward(sigs) {
		if s.Failed {
			continue
		}
		res, err := p.d.Detect.Handle(ctx, DetectExternalDeposit{
			Signature: s.Signature, CabalID: w.CabalID, Treasury: w.Address, Source: domain.SourceReconcile,
		})
		if err != nil {
			return true, recorded, err
		}
		if res.Recorded {
			recorded++
		}
	}
	return true, recorded, p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).AdvanceWatchCursor(ctx, sqlc.AdvanceWatchCursorParams{
			CabalID: w.CabalID.UUID(), LastSignature: string(sigs[0].Signature), UpdatedAt: p.d.Clock.Now(),
		})
	})
}

func (p *TreasuryReconcilePoller) hasSurplus(ctx context.Context, w cabalport.TreasuryWallet) (bool, error) {
	positions, err := p.d.Ledger.Positions(ctx, w.CabalID)
	if err != nil {
		return false, err
	}
	ledger := map[chain.SolanaAddress]money.BaseUnits{p.d.USDC: money.NewBaseUnits(0, usdcDecimals)}
	for _, pos := range positions {
		ledger[pos.Mint] = pos.Units
	}
	for mint, units := range ledger {
		onChain, err := p.d.Chain.TokenBalance(ctx, w.Address, chain.Mint{Address: mint, Decimals: units.Decimals()})
		if err != nil {
			return false, err
		}
		if onChain.Uint64() > units.Uint64() {
			return true, nil
		}
	}
	return false, nil
}

func (p *TreasuryReconcilePoller) signaturesSince(
	ctx context.Context, addr chain.SolanaAddress, cursor chain.Signature,
) ([]solana.SignatureInfo, error) {
	var out []solana.SignatureInfo
	opts := solana.SignaturesOpts{Until: cursor, Limit: reconcilePageSize}
	for page := 0; cursor != "" || page < reconcileBootstrapPage; page++ {
		got, err := p.d.Chain.SignaturesFor(ctx, addr, opts)
		if err != nil {
			return nil, err
		}
		out = append(out, got...)
		if len(got) < reconcilePageSize {
			break
		}
		opts.Before = got[len(got)-1].Signature
	}
	return out, nil
}
