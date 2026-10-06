package funding_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	treasuryport "github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type reconcileChain struct {
	mu         sync.Mutex
	balances   map[chain.SolanaAddress]map[chain.SolanaAddress]uint64
	history    map[chain.SolanaAddress][]solana.SignatureInfo
	queries    []solana.SignaturesOpts
	balanceErr error
	sigsErr    error
	down       map[chain.SolanaAddress]error
	hang       map[chain.SolanaAddress]bool
}

func (c *reconcileChain) TokenBalance(
	ctx context.Context, owner chain.SolanaAddress, mint chain.Mint,
) (money.BaseUnits, error) {
	if c.hang[owner] {
		<-ctx.Done()
		return money.BaseUnits{}, errs.Wrap(ctx.Err(), errs.CodeUpstreamTimeout, "test.hang")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.down[owner]; err != nil {
		return money.BaseUnits{}, err
	}
	return money.NewBaseUnits(c.balances[owner][mint.Address], mint.Decimals), c.balanceErr
}

func (c *reconcileChain) SignaturesFor(
	_ context.Context, addr chain.SolanaAddress, in solana.SignaturesOpts,
) ([]solana.SignatureInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.down[addr]; err != nil {
		return nil, err
	}
	c.queries = append(c.queries, in)
	if c.sigsErr != nil {
		return nil, c.sigsErr
	}
	var out []solana.SignatureInfo
	started := in.Before == ""
	for _, s := range c.history[addr] {
		if s.Signature == in.Until {
			break
		}
		if started && len(out) < in.Limit {
			out = append(out, s)
		}
		started = started || s.Signature == in.Before
	}
	return out, nil
}

type ledgerPositions map[ids.CabalID][]treasuryport.Position

func (l ledgerPositions) Positions(_ context.Context, id ids.CabalID) ([]treasuryport.Position, error) {
	return l[id], nil
}

type treasuryWallets struct {
	wallets []cabalport.TreasuryWallet
	err     error
}

func (w *treasuryWallets) TreasuryWallets(context.Context) ([]cabalport.TreasuryWallet, error) {
	return w.wallets, w.err
}

type detectorFunc func(context.Context, app.DetectExternalDeposit) (app.DetectResult, error)

func (f detectorFunc) Handle(ctx context.Context, cmd app.DetectExternalDeposit) (app.DetectResult, error) {
	return f(ctx, cmd)
}

type reconcileEnv struct {
	*watchEnv
	chain   *reconcileChain
	ledger  ledgerPositions
	wallets *treasuryWallets
	metrics *sdkmetric.ManualReader
	deps    app.TreasuryReconcileDeps
}

func newReconcileEnv(t *testing.T) *reconcileEnv {
	t.Helper()
	watch := newWatchEnv(t)
	env := &reconcileEnv{
		watchEnv: watch,
		chain: &reconcileChain{
			balances: map[chain.SolanaAddress]map[chain.SolanaAddress]uint64{},
			history:  map[chain.SolanaAddress][]solana.SignatureInfo{},
		},
		ledger:  ledgerPositions{},
		metrics: sdkmetric.NewManualReader(),
		wallets: &treasuryWallets{wallets: []cabalport.TreasuryWallet{
			{CabalID: watch.cabal.ID, Address: watch.cabal.TreasuryAddress},
		}},
	}
	failed, err := sdkmetric.NewMeterProvider(sdkmetric.WithReader(env.metrics)).Meter("test").
		Int64Counter("funding_reconcile_failed_total")
	if err != nil {
		t.Fatal(err)
	}
	env.deps = app.TreasuryReconcileDeps{
		Failed: failed,
		UoW:    db.New(watch.pool, testkit.NewIDs(60), watch.clock), Reads: watch.pool, Clock: watch.clock,
		Treasuries: env.wallets, Ledger: env.ledger, Chain: env.chain, USDC: watch.usdc,
		Detect: detectorFunc(func(ctx context.Context, cmd app.DetectExternalDeposit) (app.DetectResult, error) {
			return app.NewDetectExternalDepositHandler(watch.deps).Handle(ctx, cmd)
		}),
	}
	return env
}

func (e *reconcileEnv) onChain(owner chain.SolanaAddress, mint chain.SolanaAddress, units uint64) {
	if e.chain.balances[owner] == nil {
		e.chain.balances[owner] = map[chain.SolanaAddress]uint64{}
	}
	e.chain.balances[owner][mint] = units
}

func (e *reconcileEnv) landed(owner chain.SolanaAddress, sigs ...chain.Signature) {
	for _, sig := range sigs {
		e.chain.history[owner] = slices.Insert(e.chain.history[owner], 0, solana.SignatureInfo{Signature: sig})
	}
}

func (e *reconcileEnv) tick(t *testing.T) (int, error) {
	t.Helper()
	report, err := e.tickReport(t)
	return report.Changed, err
}

func (e *reconcileEnv) tickReport(t *testing.T) (poller.Report, error) {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:poller.funding.treasury-reconcile")
	return app.NewTreasuryReconcilePoller(e.deps).Tick(ctx)
}

func attr(report poller.Report, key string) int64 {
	for _, a := range report.Attrs {
		if a.Key == key {
			return a.Value.Int64()
		}
	}
	return -1
}

func (e *reconcileEnv) failures(t *testing.T) map[string]int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := e.metrics.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	out := map[string]int64{}
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != "funding_reconcile_failed_total" {
				continue
			}
			for _, dp := range sum.DataPoints {
				code, _ := dp.Attributes.Value(attribute.Key("code"))
				out[code.AsString()] += dp.Value
			}
		}
	}
	return out
}

func (e *reconcileEnv) cursor(t *testing.T) string {
	t.Helper()
	var sig string
	_ = e.pool.QueryRow(t.Context(), `SELECT last_signature FROM treasury_watch_cursors WHERE cabal_id = $1`,
		e.cabal.ID.UUID()).Scan(&sig)
	return sig
}

func TestTreasuryReconcile_NoSurplusReadsNoSignatures(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	treasury := env.cabal.TreasuryAddress
	env.ledger[env.cabal.ID] = []treasuryport.Position{
		{Mint: env.usdc, Units: money.NewBaseUnits(10_000_000, 6)},
		{Mint: env.stock.Mint.Address(), Units: money.NewBaseUnits(3, 8)},
	}
	env.onChain(treasury, env.usdc, 10_000_000)
	env.onChain(treasury, env.stock.Mint.Address(), 3)
	env.landed(treasury, env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t)))

	changed, err := env.tick(t)

	if err != nil || changed != 0 || len(env.chain.queries) != 0 || env.cursor(t) != "" {
		t.Fatalf("tick = %d, %v after %d signature reads, want nothing", changed, err, len(env.chain.queries))
	}
}

func TestTreasuryReconcile_SurplusRecordsOnlyTheStrayTransfer(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	treasury := env.cabal.TreasuryAddress
	sweep := env.inbound(t, env.usdcMint(), 20_000_000, randomAddress(t))
	env.owned[sweep] = true
	stray := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.landed(treasury, sweep, stray)
	env.chain.history[treasury] = append(env.chain.history[treasury],
		solana.SignatureInfo{Signature: "failed", Failed: true})
	env.ledger[env.cabal.ID] = []treasuryport.Position{{Mint: env.usdc, Units: money.NewBaseUnits(20_000_000, 6)}}
	env.onChain(treasury, env.usdc, 25_000_000)

	changed, err := env.tick(t)

	if err != nil || changed != 1 {
		t.Fatalf("tick = %d, %v, want one recorded", changed, err)
	}
	rows := externalDeposits(t, env.pool)
	if len(rows) != 1 || rows[string(stray)].source != "reconcile" || rows[string(stray)].status != "detected" {
		t.Fatalf("rows = %+v, want only the stray transfer from reconcile", rows)
	}
	if env.cursor(t) != string(stray) || externalPauses(t, env.pool) != 1 {
		t.Fatalf("cursor = %q, want the newest signature %q and one pause", env.cursor(t), stray)
	}
}

func TestTreasuryReconcile_ReadsBackToTheCursorPageByPage(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	treasury := env.cabal.TreasuryAddress
	env.landed(treasury, "seen")
	env.changed(t, treasury)
	for i := range 60 {
		env.landed(treasury, chain.Signature(fmt.Sprintf("noise-%02d", i)))
	}
	stray := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.landed(treasury, stray)
	env.onChain(treasury, env.usdc, 5_000_000)

	changed, err := env.tick(t)

	if err != nil || changed != 1 || env.cursor(t) != string(stray) {
		t.Fatalf(
			"tick = %d, %v, cursor %q, want one recorded and the cursor at the newest",
			changed,
			err,
			env.cursor(t),
		)
	}
	want := []solana.SignaturesOpts{
		{Until: "seen", Limit: 50},
		{Until: "seen", Before: "noise-11", Limit: 50},
	}
	if !slices.Equal(env.chain.queries, want) {
		t.Fatalf("signature reads = %+v, want %+v", env.chain.queries, want)
	}
}

func (e *reconcileEnv) changed(t *testing.T, treasury chain.SolanaAddress) {
	t.Helper()
	exec(t, e.pool, fmt.Sprintf(`INSERT INTO treasury_watch_cursors (cabal_id, last_signature, updated_at)
		VALUES ('%s', '%s', now())`, e.cabal.ID, e.chain.history[treasury][0].Signature))
}

func TestTreasuryReconcile_AFailingTreasuryDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	broken := testkit.NewCabal(t, env.pool)
	env.wallets.wallets = append(env.wallets.wallets,
		cabalport.TreasuryWallet{CabalID: broken.ID, Address: broken.TreasuryAddress})
	treasury := env.cabal.TreasuryAddress
	stray := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.landed(treasury, stray)
	env.onChain(treasury, env.usdc, 5_000_000)
	env.onChain(broken.TreasuryAddress, env.usdc, 1)
	env.landed(broken.TreasuryAddress, "unreadable")
	env.watchEnv.deps.Chain = &failOn{sig: "unreadable", next: env.watchEnv.chain}

	changed, err := env.tick(t)

	if err != nil || changed != 1 || env.cursor(t) != string(stray) {
		t.Fatalf("tick = %d, %v, want no tick error and the healthy cabal recorded", changed, err)
	}
	if got := env.failures(t); got["upstream_unavailable"]+got["internal"] != 1 {
		t.Fatalf("failures = %v, want the broken cabal counted once", got)
	}
	var brokenCursor int
	if err := env.pool.QueryRow(t.Context(), `SELECT count(*) FROM treasury_watch_cursors WHERE cabal_id = $1`,
		broken.ID.UUID()).Scan(&brokenCursor); err != nil || brokenCursor != 0 {
		t.Fatalf("broken cabal cursor rows = %d, %v, want none so the next tick retries", brokenCursor, err)
	}
}

type failOn struct {
	sig  chain.Signature
	next app.WatchChain
}

func (f *failOn) InboundTransfers(
	ctx context.Context, sig chain.Signature, owner chain.SolanaAddress,
) ([]solana.Transfer, error) {
	if sig == f.sig {
		return nil, errWatchDown
	}
	return f.next.InboundTransfers(ctx, sig, owner)
}

func TestTreasuryReconcile_AnRPCFailureOnOneCabalStillReconcilesTheOthers(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	broken := testkit.NewCabal(t, env.pool)
	env.wallets.wallets = append(env.wallets.wallets,
		cabalport.TreasuryWallet{CabalID: broken.ID, Address: broken.TreasuryAddress})
	env.chain.down = map[chain.SolanaAddress]error{
		broken.TreasuryAddress: errs.New(errs.CodeRPCUnavailable, "test.rpc"),
	}
	treasury := env.cabal.TreasuryAddress
	stray := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.landed(treasury, stray)
	env.onChain(treasury, env.usdc, 5_000_000)

	report, err := env.tickReport(t)

	if err != nil || report.Changed != 1 || env.cursor(t) != string(stray) {
		t.Fatalf("tick = %+v, %v, want the healthy cabal recorded and no tick error", report, err)
	}
	if report.Scanned != 2 || attr(report, "failed") != 1 || attr(report, "unreached") != 0 {
		t.Fatalf("report = %+v, want both cabals scanned and one failure", report)
	}
	if got := env.failures(t); got[string(errs.CodeRPCUnavailable)] != 1 || len(got) != 1 {
		t.Fatalf("failures = %v, want one rpc_unavailable", got)
	}
}

func TestTreasuryReconcile_CabalsCutOffByTheTickBudgetGoLastNextTick(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	env.deps.Interval = 300 * time.Millisecond
	env.chain.hang = map[chain.SolanaAddress]bool{}
	for range 10 {
		slow := testkit.NewCabal(t, env.pool)
		env.chain.hang[slow.TreasuryAddress] = true
		env.wallets.wallets = append(env.wallets.wallets,
			cabalport.TreasuryWallet{CabalID: slow.ID, Address: slow.TreasuryAddress})
	}
	treasury := env.cabal.TreasuryAddress
	stray := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.landed(treasury, stray)
	env.onChain(treasury, env.usdc, 5_000_000)
	reconcile := app.NewTreasuryReconcilePoller(env.deps)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.treasury-reconcile")

	first, err := reconcile.Tick(ctx)
	if err != nil || first.Changed != 0 || attr(first, "unreached") != 11 || attr(first, "failed") != 0 {
		t.Fatalf("first tick = %+v, %v, want every cabal unreached behind the slow ones", first, err)
	}
	second, err := reconcile.Tick(ctx)

	if err != nil || second.Changed != 1 || env.cursor(t) != string(stray) {
		t.Fatalf("second tick = %+v, %v, want the cabal the first tick never started reconciled", second, err)
	}
	if got := env.failures(t); len(got) != 0 {
		t.Fatalf("failures = %v, want cut-off cabals not counted as failed", got)
	}
}

func TestTreasuryReconcile_ACabalReadFailureIsCountedNotFatal(t *testing.T) {
	t.Parallel()
	cases := map[string]func(t *testing.T, e *reconcileEnv){
		"balance":    func(_ *testing.T, e *reconcileEnv) { e.chain.balanceErr = errWatchDown },
		"signatures": func(_ *testing.T, e *reconcileEnv) { e.chain.sigsErr = errWatchDown },
		"ledger": func(_ *testing.T, e *reconcileEnv) {
			e.deps.Ledger = failingLedger{}
		},
		"cursor": func(t *testing.T, e *reconcileEnv) {
			t.Helper()
			exec(t, e.pool, `ALTER TABLE treasury_watch_cursors RENAME TO treasury_watch_cursors_gone`)
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			env := newReconcileEnv(t)
			env.onChain(env.cabal.TreasuryAddress, env.usdc, 5_000_000)
			arrange(t, env)
			report, err := env.tickReport(t)
			if err != nil || report.Scanned != 1 || attr(report, "failed") != 1 {
				t.Fatalf("tick = %+v, %v, want one failed cabal and no tick error", report, err)
			}
			if got := env.failures(t); len(got) != 1 {
				t.Fatalf("failures = %v, want one counted", got)
			}
		})
	}
}

func TestTreasuryReconcile_AFailedTreasuryListFailsTheTick(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	env.wallets.err = errWatchDown
	if _, err := env.tick(t); err == nil {
		t.Fatal("tick = nil error")
	}
}

type failingLedger struct{}

func (failingLedger) Positions(context.Context, ids.CabalID) ([]treasuryport.Position, error) {
	return nil, errWatchDown
}

func TestTreasuryReconcile_NothingLandedLeavesTheCursorAlone(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	env.onChain(env.cabal.TreasuryAddress, env.usdc, 5_000_000)

	changed, err := env.tick(t)

	if err != nil || changed != 0 || env.cursor(t) != "" || len(env.chain.queries) != 1 {
		t.Fatalf("tick = %d, %v, want one empty read and no cursor", changed, err)
	}
}

func TestTreasuryReconcile_ACancelledTickFails(t *testing.T) {
	t.Parallel()
	env := newReconcileEnv(t)
	ctx, cancel := context.WithCancel(observability.WithActor(t.Context(), "system:test"))
	cancel()
	if _, err := app.NewTreasuryReconcilePoller(env.deps).Tick(ctx); err == nil {
		t.Fatal("cancelled tick = nil error")
	}
}
