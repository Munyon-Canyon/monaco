package app

import (
	"context"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"golang.org/x/sync/errgroup"

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
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	reconcileFanOut        = 8
	reconcilePageSize      = 50
	reconcileBootstrapPage = 20
	usdcDecimals           = 6
	reconcileBudgetPercent = 80
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
	SolanaPositions(ctx context.Context, cabalID ids.CabalID) ([]treasuryport.Position, error)
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
	Failed     metric.Int64Counter
}

type TreasuryReconcilePoller struct {
	d TreasuryReconcileDeps

	mu      sync.Mutex
	pass    int
	reached map[ids.CabalID]int
	short   map[shortKey]bool
}

type shortKey struct {
	cabal ids.CabalID
	mint  chain.SolanaAddress
}

type deficit struct {
	mint    chain.SolanaAddress
	ledger  money.BaseUnits
	onChain money.BaseUnits
}

func NewTreasuryReconcilePoller(d TreasuryReconcileDeps) *TreasuryReconcilePoller {
	return &TreasuryReconcilePoller{d: d, reached: map[ids.CabalID]int{}, short: map[shortKey]bool{}}
}

func (*TreasuryReconcilePoller) Name() string { return "funding.treasury-reconcile" }

func (p *TreasuryReconcilePoller) Interval() time.Duration { return p.d.Interval }

type reconciled struct {
	started  bool
	cut      bool
	surplus  bool
	deficits int
	recorded int
	err      error
}

func (p *TreasuryReconcilePoller) Tick(ctx context.Context) (poller.Report, error) {
	const op = "funding.TreasuryReconcile.Tick"
	wallets, err := p.d.Treasuries.TreasuryWallets(ctx)
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), op)
	}
	wallets = p.order(wallets)
	work, cancel := p.budgeted(ctx)
	defer cancel()
	results, err := concurrency.FanOut(ctx, reconcileFanOut, wallets,
		func(_ context.Context, w cabalport.TreasuryWallet) (reconciled, error) {
			return p.visit(work, w), nil
		})
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	return p.report(ctx, wallets, results), nil
}

func (p *TreasuryReconcilePoller) visit(work context.Context, w cabalport.TreasuryWallet) reconciled {
	if work.Err() != nil {
		return reconciled{}
	}
	surplus, deficits, recorded, err := p.reconcile(work, w)
	cut := err != nil && work.Err() != nil
	return reconciled{started: true, cut: cut, surplus: surplus, deficits: deficits, recorded: recorded, err: err}
}

func (p *TreasuryReconcilePoller) order(wallets []cabalport.TreasuryWallet) []cabalport.TreasuryWallet {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := slices.Clone(wallets)
	slices.SortStableFunc(out, func(a, b cabalport.TreasuryWallet) int {
		if pa, pb := p.reached[a.CabalID], p.reached[b.CabalID]; pa != pb {
			return pa - pb
		}
		return strings.Compare(b.CabalID.String(), a.CabalID.String())
	})
	return out
}

func (p *TreasuryReconcilePoller) report(
	ctx context.Context, wallets []cabalport.TreasuryWallet, results []reconciled,
) poller.Report {
	p.mu.Lock()
	p.pass++
	live := make(map[ids.CabalID]int, len(wallets))
	for i, w := range wallets {
		live[w.CabalID] = p.reached[w.CabalID]
		if results[i].started {
			live[w.CabalID] = p.pass
		}
	}
	p.reached = live
	p.mu.Unlock()
	report := poller.Report{}
	var surpluses, deficits, failed int
	for i, r := range results {
		if !r.started || r.cut {
			continue
		}
		report.Scanned++
		report.Changed += r.recorded
		if r.surplus {
			surpluses++
		}
		deficits += r.deficits
		if r.err != nil {
			failed++
			p.failed(ctx, wallets[i].CabalID, r.err)
		}
	}
	report.Attrs = []slog.Attr{
		slog.Int("surpluses", surpluses), slog.Int("deficits", deficits), slog.Int("failed", failed),
		slog.Int("unreached", len(wallets)-report.Scanned),
	}
	return report
}

func (p *TreasuryReconcilePoller) failed(ctx context.Context, cabal ids.CabalID, err error) {
	code := string(errs.CodeOf(err))
	observability.Degraded(ctx, observability.FundingReconcileFailed,
		slog.String("cabal_id", cabal.String()), slog.String("code", code), slog.Any("err", err))
	p.d.Failed.Add(ctx, 1, metric.WithAttributes(attribute.String("code", code)))
}

func (p *TreasuryReconcilePoller) budgeted(ctx context.Context) (context.Context, context.CancelFunc) {
	if p.d.Interval <= 0 {
		return context.WithCancel(ctx)
	}
	work, cancel := context.WithCancel(ctx)
	expired := p.d.Clock.After(p.d.Interval * reconcileBudgetPercent / 100)
	var watch errgroup.Group
	watch.Go(func() error {
		select {
		case <-expired:
			cancel()
		case <-work.Done():
		}
		return nil
	})
	return work, func() {
		cancel()
		_ = watch.Wait()
	}
}

func (p *TreasuryReconcilePoller) reconcile(
	ctx context.Context, w cabalport.TreasuryWallet,
) (bool, int, int, error) {
	const op = "funding.TreasuryReconcile.reconcile"
	surplus, shorts, err := p.compare(ctx, w)
	if err != nil {
		return false, 0, 0, err
	}
	p.alertDeficits(ctx, w.CabalID, shorts)
	if !surplus {
		return false, len(shorts), 0, nil
	}
	cursor, err := sqlc.New(p.d.Reads).WatchCursor(ctx, w.CabalID.UUID())
	if err != nil {
		return true, len(shorts), 0, errs.Wrap(err, errs.CodeInternal, op)
	}
	sigs, err := p.signaturesSince(ctx, w.Address, chain.Signature(cursor))
	if err != nil || len(sigs) == 0 {
		return true, len(shorts), 0, err
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
			return true, len(shorts), recorded, err
		}
		if res.Recorded {
			recorded++
		}
	}
	return true, len(shorts), recorded, p.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		return sqlc.New(tx.Queries()).AdvanceWatchCursor(ctx, sqlc.AdvanceWatchCursorParams{
			CabalID: w.CabalID.UUID(), LastSignature: string(sigs[0].Signature), UpdatedAt: p.d.Clock.Now(),
		})
	})
}

func (p *TreasuryReconcilePoller) compare(
	ctx context.Context, w cabalport.TreasuryWallet,
) (bool, []deficit, error) {
	positions, err := p.d.Ledger.SolanaPositions(ctx, w.CabalID)
	if err != nil {
		return false, nil, err
	}
	ledger := map[chain.SolanaAddress]money.BaseUnits{p.d.USDC: money.NewBaseUnits(0, usdcDecimals)}
	for _, pos := range positions {
		ledger[pos.Mint] = pos.Units
	}
	surplus := false
	var shorts []deficit
	for mint, units := range ledger {
		onChain, err := p.d.Chain.TokenBalance(ctx, w.Address, chain.Mint{Address: mint, Decimals: units.Decimals()})
		if err != nil {
			return false, nil, err
		}
		switch {
		case onChain.Uint64() > units.Uint64():
			surplus = true
		case onChain.Uint64() < units.Uint64():
			shorts = append(shorts, deficit{mint: mint, ledger: units, onChain: onChain})
		}
	}
	return surplus, shorts, nil
}

func (p *TreasuryReconcilePoller) alertDeficits(ctx context.Context, cabal ids.CabalID, shorts []deficit) {
	p.mu.Lock()
	persisted := make([]deficit, 0, len(shorts))
	now := make(map[chain.SolanaAddress]bool, len(shorts))
	for _, d := range shorts {
		now[d.mint] = true
		if p.short[shortKey{cabal: cabal, mint: d.mint}] {
			persisted = append(persisted, d)
		}
	}
	for k := range p.short {
		if k.cabal == cabal && !now[k.mint] {
			delete(p.short, k)
		}
	}
	for _, d := range shorts {
		p.short[shortKey{cabal: cabal, mint: d.mint}] = true
	}
	p.mu.Unlock()
	for _, d := range persisted {
		observability.Alert(ctx, observability.FundingReconcileDeficit,
			slog.String("cabal_id", cabal.String()), slog.String("mint", string(d.mint)),
			slog.String("ledger_units", strconv.FormatUint(d.ledger.Uint64(), 10)),
			slog.String("chain_units", strconv.FormatUint(d.onChain.Uint64(), 10)))
	}
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
