package market_test

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"math"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type seeded struct {
	symbol   string
	tradable bool
}

type noMintFacts struct{}

func (noMintFacts) Facts(context.Context, []domain.Mint) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	return map[domain.Mint]app.MintFact{}, map[domain.Mint]error{}, nil
}

type partialMintFacts struct {
	facts map[domain.Mint]app.MintFact
	err   error
}

func (f partialMintFacts) Facts(
	context.Context, []domain.Mint,
) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	return f.facts, map[domain.Mint]error{}, f.err
}

func TestCatalogPoller_refusesAMissingFact(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	rig.seed(t, seeded{symbol: "AAPLx", tradable: true})
	if _, err := rig.asking(noMintFacts{}).Tick(rig.ctx(t)); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("Tick = %v, want decode_failed", err)
	}
}

func TestCatalogPoller_keepsTheFirstBatchWhenTheSecondFails(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	assets := make([]seeded, 200)
	for i := range assets {
		assets[i] = seeded{symbol: fmt.Sprintf("S%03dx", i), tradable: true}
	}
	rig.seed(t, assets...)
	facts := make(map[domain.Mint]app.MintFact, 100)
	for i := range 100 {
		asset := rig.asset(t, fmt.Sprintf("S%03dx", i))
		facts[asset.Mint] = app.MintFact{Decimals: 8, MultiplierNum: 1, MultiplierDen: 1}
	}
	report, err := rig.asking(partialMintFacts{
		facts: facts,
		err:   errs.New(errs.CodeRPCUnavailable, "second chunk failed"),
	}).Tick(rig.ctx(t))
	unchecked := rig.unchecked(t)
	if report.Changed != 100 || len(unchecked) != 100 || errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Tick = %+v, unchecked = %d, err = %v; want 100, 100, rpc_unavailable", report, len(unchecked), err)
	}
	firstMint := rig.asset(t, "S100x").Mint.String()
	details := errs.Detail(err)
	has := func(attr slog.Attr) bool {
		return slices.ContainsFunc(details, func(got slog.Attr) bool { return got.Equal(attr) })
	}
	if !has(slog.Int("failed", 100)) || !has(slog.Int("checked", 100)) || !has(slog.String("first_mint", firstMint)) {
		t.Fatalf("error details = %v, want failed=100 checked=100 first_mint=%s", details, firstMint)
	}
}

func (r *catalogRig) seed(t *testing.T, assets ...seeded) {
	t.Helper()
	ids := make([]uuid.UUID, len(assets))
	symbols, mints, tradables := make([]string, len(assets)), make([]string, len(assets)), make([]bool, len(assets))
	for i, a := range assets {
		sum := sha256.Sum256([]byte(a.symbol))
		m := mint(t, string(chain.AddressOf(sum[:])))
		r.facts.Put(m, 8, 1, 1)
		ids[i], symbols[i], mints[i], tradables[i] = domain.NewAssetID(r.ids).UUID(), a.symbol, m.String(), a.tradable
	}
	r.exec(t, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
		company_key, first_seen_at, updated_at)
		SELECT u.id, u.symbol, u.mint, 8, 'xstocks', 'equity', u.symbol, u.tradable, lower(u.symbol), now(), now()
		FROM unnest($1::uuid[], $2::text[], $3::text[], $4::bool[]) AS u (id, symbol, mint, tradable)`,
		ids, symbols, mints, tradables)
}

func (r *catalogRig) unchecked(t *testing.T) []string {
	t.Helper()
	all, err := r.catalog.ListAll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, a := range all {
		if !a.ChainChecked {
			out = append(out, a.Symbol)
		}
	}
	return out
}

func TestCatalogPoller_storesTheChainsDecimalsAndMultiplierAndLogsTheCorrection(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	xs.serve(nil, listed(aapl), listed(tsla))
	rig := newRig(t, xs)
	rig.facts.Put(aapl.Mint, 6, 1, 1)
	rig.facts.Put(tsla.Mint, 8, 10_032_690_125_398_187, 10_000_000_000_000_000)
	if got := rig.tick(t); got.Changed != 4 {
		t.Fatalf("first tick = %+v, want 2 inserted and 2 checked", got)
	}
	if a := rig.asset(t, "AAPLx"); a.Decimals != 6 || !a.ChainChecked || !a.Tradable() {
		t.Fatalf("AAPLx = %+v, want the chain's 6 decimals over the issuer's 8, checked and tradable", a)
	}
	multiplier := domain.Multiplier{Num: 10_032_690_125_398_187, Den: 10_000_000_000_000_000}
	if a := rig.asset(t, "TSLAx"); a.Decimals != 8 || a.UIMultiplier != multiplier || !a.ChainChecked {
		t.Fatalf("TSLAx = %+v, want 8 decimals and the chain's multiplier %+v", a, multiplier)
	}
	if got := rig.tradable(t); !slices.Equal(got, []string{"AAPLx", "TSLAx"}) {
		t.Fatalf("tradable = %v, want both checked assets", got)
	}
	if got, want := rig.corrections(t), []string{correction(aapl, 8, 6)}; !slices.Equal(got, want) {
		t.Fatalf("corrections = %v, want only AAPLx's %v", got, want)
	}
}

func TestCatalogPoller_anRPCFailureLeavesThatMintUncheckedUntilTheNextTick(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	tsla := marketfake.TSLAx()
	xs.serve(nil, listed(marketfake.AAPLx()), listed(tsla), listed(marketfake.JPSTx()))
	rig := newRig(t, xs)
	rig.facts.FailOnce("Facts:"+tsla.Mint.String(), errs.New(errs.CodeRPCUnavailable, "test"))
	report, err := rig.poller.Tick(rig.ctx(t))
	names := func(a slog.Attr) bool { return a.Equal(slog.String("symbol", "TSLAx")) }
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || !slices.ContainsFunc(errs.Detail(err), names) {
		t.Fatalf("Tick = %v, want rpc_unavailable naming TSLAx", err)
	}
	if report.Changed != 5 {
		t.Fatalf("failed tick = %+v, want 3 inserted and the other 2 checked", report)
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"TSLAx"}) {
		t.Fatalf("unchecked = %v, want only TSLAx", got)
	}
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 1 || len(rig.unchecked(t)) != 0 || rig.facts.Asked() != 4 {
		t.Fatalf("retry tick = %+v, unchecked %v after %d asks, want TSLAx asked again and checked", got,
			rig.unchecked(t), rig.facts.Asked())
	}
}

func TestCatalogPoller_checksAtMost200MintsATickTradableFirst(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	assets := make([]seeded, 0, 201)
	assets = append(assets, seeded{symbol: "AAAx"})
	for i := range 200 {
		assets = append(assets, seeded{symbol: fmt.Sprintf("S%03dx", i), tradable: true})
	}
	rig.seed(t, assets...)
	if got := rig.tick(t); got.Changed != 200 || rig.facts.Asked() != 200 {
		t.Fatalf("first tick = %+v after %d asks, want 200 checked", got, rig.facts.Asked())
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"AAAx"}) {
		t.Fatalf("unchecked = %v, want the halted AAAx left for the next tick", got)
	}
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 1 || len(rig.unchecked(t)) != 0 || rig.facts.Asked() != 201 {
		t.Fatalf("second tick = %+v after %d asks, want AAAx checked", got, rig.facts.Asked())
	}
}

type gate struct {
	facts app.MintFacts

	mu    sync.Mutex
	asked int
}

func (g *gate) Facts(
	ctx context.Context, mints []domain.Mint,
) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	g.mu.Lock()
	g.asked = len(mints)
	g.mu.Unlock()
	return g.facts.Facts(ctx, mints)
}

func (g *gate) count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.asked
}

func TestCatalogPoller_asksTheChainAboutEveryUncheckedMintAtOnce(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	assets := make([]seeded, 20)
	for i := range assets {
		assets[i] = seeded{symbol: fmt.Sprintf("S%02dx", i), tradable: true}
	}
	rig.seed(t, assets...)
	g := &gate{facts: rig.facts}
	report, err := rig.asking(g).Tick(rig.ctx(t))
	if err != nil || g.count() != 20 || report.Changed != 20 {
		t.Fatalf("asked %d mints, tick %+v, %v; want one 20-mint call and all checked", g.count(), report, err)
	}
}

func TestCatalogPoller_aMultiplierTooLargeToStoreLeavesThatMintUnchecked(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	xs.serve(nil, listed(marketfake.AAPLx()), listed(marketfake.TSLAx()))
	rig := newRig(t, xs)
	rig.facts.Put(marketfake.TSLAx().Mint, 8, math.MaxUint64, 1)
	report, err := rig.poller.Tick(rig.ctx(t))
	if errs.CodeOf(err) != errs.CodeDecodeFailed || report.Changed != 3 {
		t.Fatalf("Tick = %+v, %v, want decode_failed with AAPLx checked", report, err)
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"TSLAx"}) {
		t.Fatalf("unchecked = %v, want only TSLAx", got)
	}
}

func TestCatalogPoller_aFailedStoreLeavesEveryMintUncheckedForTheNextTick(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl := marketfake.AAPLx()
	xs.serve(nil, listed(aapl), listed(marketfake.TSLAx()))
	rig := newRig(t, xs)
	rig.facts.Put(aapl.Mint, 6, 1, 1)
	rig.exec(t, `ALTER TABLE assets ADD CONSTRAINT refuse_checks CHECK (chain_checked_at IS NULL)`)
	if _, err := rig.poller.Tick(rig.ctx(t)); err == nil {
		t.Fatal("Tick with the store refused succeeded")
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"AAPLx", "TSLAx"}) || len(rig.corrections(t)) != 0 {
		t.Fatalf("unchecked = %v, corrections %v, want both unchecked and no correction logged for a rolled-back store",
			got, rig.corrections(t))
	}
	rig.exec(t, `ALTER TABLE assets DROP CONSTRAINT refuse_checks`)
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 2 || len(rig.unchecked(t)) != 0 {
		t.Fatalf("next tick = %+v, unchecked %v, want both checked", got, rig.unchecked(t))
	}
	if got, want := rig.corrections(t), []string{correction(aapl, 8, 6)}; !slices.Equal(got, want) {
		t.Fatalf("corrections = %v, want AAPLx's %v once, from the tick that stored it", got, want)
	}
}

type interleaving struct {
	app.MintFacts

	mu     sync.Mutex
	before map[domain.Mint]func()
}

func (f *interleaving) Facts(
	ctx context.Context, mints []domain.Mint,
) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	for _, mint := range mints {
		f.mu.Lock()
		write := f.before[mint]
		delete(f.before, mint)
		f.mu.Unlock()
		if write != nil {
			write()
		}
	}
	return f.MintFacts.Facts(ctx, mints)
}

func TestCatalogPoller_leavesARowThatMovedWhileTheChainWasAsked(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	xs.serve(nil, listed(aapl), listed(tsla))
	rig := newRig(t, xs)
	rig.facts.Put(aapl.Mint, 6, 1, 1)
	rig.facts.Put(tsla.Mint, 6, 1, 1)
	write := func(sql string) func() {
		return func() {
			if _, err := rig.pool.Exec(t.Context(), sql); err != nil {
				t.Error(err)
			}
		}
	}
	reissued := listed(tsla)
	reissued.Decimals = 9
	refresh := write(`UPDATE assets SET decimals = 9 WHERE symbol = 'TSLAx'`)
	racing := rig.asking(&interleaving{MintFacts: rig.facts, before: map[domain.Mint]func(){
		aapl.Mint: write(`UPDATE assets SET ui_multiplier_num = 3, ui_multiplier_den = 2, chain_checked_at = now()
			WHERE symbol = 'AAPLx'`),
		tsla.Mint: func() {
			xs.serve(nil, listed(aapl), reissued)
			refresh()
		},
	}})
	if got, err := racing.Tick(rig.ctx(t)); err != nil || got.Changed != 2 {
		t.Fatalf("Tick = %+v, %v, want the 2 inserts and neither moved row stored", got, err)
	}
	if a := rig.asset(t, "AAPLx"); a.Decimals != 8 || a.UIMultiplier != (domain.Multiplier{Num: 3, Den: 2}) {
		t.Fatalf("AAPLx = %+v, want the check that landed first kept", a)
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"TSLAx"}) || len(rig.corrections(t)) != 0 {
		t.Fatalf("unchecked = %v, corrections %v, want TSLAx left for the next tick and nothing logged", got,
			rig.corrections(t))
	}
	rig.clock.Advance(time.Hour)
	rig.tick(t)
	if got, want := rig.corrections(t), []string{correction(tsla, 9, 6)}; !slices.Equal(got, want) {
		t.Fatalf("corrections = %v, want TSLAx's %v against the decimals it moved to", got, want)
	}
}

type cancelling struct {
	app.MintFacts
	cancel context.CancelFunc
}

func (c cancelling) Facts(
	ctx context.Context, mints []domain.Mint,
) (map[domain.Mint]app.MintFact, map[domain.Mint]error, error) {
	c.cancel()
	return c.MintFacts.Facts(ctx, mints)
}

func TestCatalogPoller_aTickCancelledWhileAskingTheChainStoresNothing(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	xs.serve(nil, listed(marketfake.AAPLx()))
	rig := newRig(t, xs)
	ctx, cancel := context.WithCancel(rig.ctx(t))
	defer cancel()
	if _, err := rig.asking(cancelling{MintFacts: rig.facts, cancel: cancel}).Tick(ctx); err == nil {
		t.Fatal("Tick cancelled while asking the chain succeeded")
	}
	if got := rig.unchecked(t); !slices.Equal(got, []string{"AAPLx"}) {
		t.Fatalf("unchecked = %v, want AAPLx left for the next tick", got)
	}
}

func TestCatalogPoller_anUncheckedRowThatDoesNotParseFailsTheCheck(t *testing.T) {
	t.Parallel()
	rig := newRig(t)
	rig.exec(t, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable,
		company_key, first_seen_at, updated_at)
		VALUES ($1, 'BADx', 'not-a-mint', 8, 'xstocks', 'equity', 'Bad xStock', true, 'bad', now(), now())`,
		domain.NewAssetID(rig.ids).UUID())
	if _, err := rig.poller.Tick(rig.ctx(t)); errs.CodeOf(err) != errs.CodeDecodeFailed || rig.facts.Asked() != 0 {
		t.Fatalf("Tick = %v after %d asks, want decode_failed before asking the chain", err, rig.facts.Asked())
	}
}
