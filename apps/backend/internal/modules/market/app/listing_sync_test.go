package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const (
	aaplC      = "0xb200000000000000000000c2e324d24d7eecd1fb"
	tslaC      = "0xb2000000000000000000001e800a7f5189430cd0"
	aaplRH     = "0xaf3d76f1834a1d425780943c99ea8a608f8a93f9"
	nvdaRH     = "0xd0601ce157db5bdc3162bbac2a2c8af5320d9eec"
	aaplOn     = "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c"
	aaplXEVM   = "0x9d275685dc284c8eb1c79f6aba7a63dc75ec890a"
	aaplXWrap  = "0x943bf64d566c32a2bcd41ac92fb63c111cc9de8f"
	aaplXSolan = "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"
)

type source struct {
	issuer string
	batch  app.ListingBatch
	err    error
}

func (s *source) Issuer() string { return s.issuer }

func (s *source) Listings(context.Context) (app.ListingBatch, error) { return s.batch, s.err }

type verifier struct {
	failing  map[string]bool
	decimals map[string]uint8
	cancel   context.CancelFunc
}

func (v *verifier) Verify(_ context.Context, l app.ListingToVerify) (uint8, error) {
	if v.cancel != nil {
		v.cancel()
	}
	if v.failing[l.TokenAddress] {
		return 0, errs.Wrap(fmt.Errorf("%s symbol() differs", l.TokenAddress), errs.CodeInvalidAddress, "test.Verify")
	}
	if d, ok := v.decimals[l.TokenAddress]; ok {
		return d, nil
	}
	if l.Decimals != nil {
		return *l.Decimals, nil
	}
	return 18, nil
}

type listingRig struct {
	pool   sqlc.DBTX
	uow    *db.UnitOfWork
	clock  *testkit.Clock
	logs   *testkit.Logs
	verify *verifier
	sync   *app.ListingSync
}

func newListingRig(t *testing.T, sources ...app.ListingSource) *listingRig {
	t.Helper()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM asset_listings`); err != nil {
		t.Fatal(err)
	}
	g := testkit.NewIDs(4969)
	r := &listingRig{
		pool: pool, clock: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond)),
		logs: &testkit.Logs{}, verify: &verifier{failing: map[string]bool{}},
	}
	r.uow = db.New(pool, g, r.clock)
	r.sync = app.NewListingSync(r.uow, g, r.clock, sources, r.verify)
	return r
}

func (r *listingRig) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs))
}

func (r *listingRig) run(t *testing.T) int {
	t.Helper()
	changed, err := r.sync.Run(r.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	return changed
}

func (r *listingRig) rows(t *testing.T) []string {
	t.Helper()
	rows, err := r.pool.Query(t.Context(), `SELECT issuer, symbol, network, token_address,
		coalesce(wrapper_address, ''), coalesce(decimals, -1), shares_per_token::text, issuer_tradable, enabled,
		verified_at IS NOT NULL
		FROM asset_listings ORDER BY issuer, symbol, network`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var issuer, symbol, network, token, wrapper, shares string
		var decimals int16
		var tradable, enabled, verified bool
		if err := rows.Scan(&issuer, &symbol, &network, &token, &wrapper, &decimals, &shares, &tradable, &enabled,
			&verified); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%s %s %s %s %s %d %s tradable=%t enabled=%t verified=%t", issuer, symbol,
			network, token, wrapper, decimals, strings.TrimRight(strings.TrimRight(shares, "0"), "."), tradable,
			enabled, verified))
	}
	return out
}

func (r *listingRig) logged(t *testing.T, msg string) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(string(r.logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) != nil || got["msg"] != msg {
			continue
		}
		out = append(out, fmt.Sprintf("%v %v %v %v %v", got["level"], got["issuer"], got["symbol"], got["network"],
			got["reason"]))
	}
	return out
}

func (r *listingRig) id(t *testing.T, token string) string {
	t.Helper()
	var id string
	if err := r.pool.QueryRow(t.Context(), `SELECT id::text FROM asset_listings WHERE token_address = $1`,
		token).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func rat(t *testing.T, decimal string) *big.Rat {
	t.Helper()
	r, ok := new(big.Rat).SetString(decimal)
	if !ok {
		t.Fatalf("rat %q", decimal)
	}
	return r
}

func mustUUID(t *testing.T, raw string) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func u8(n uint8) *uint8 { return &n }

func evm(issuer, ticker, symbol string, n chain.Network, token string) app.IssuerListing {
	return app.IssuerListing{
		Issuer: issuer, Ticker: ticker, Symbol: symbol, Network: n, ChainID: n.EVMChainID(), TokenAddress: token,
		Tradable: true,
	}
}

func launchSources(t *testing.T, now time.Time) []app.ListingSource {
	t.Helper()
	aapl := evm("coinbase", "AAPL", "AAPLc", chain.NetworkBase, aaplC)
	aapl.Decimals, aapl.SharesPerToken = u8(8), rat(t, "1")
	tsla := evm("coinbase", "TSLA", "TSLAc", chain.NetworkBase, tslaC)
	tsla.Decimals, tsla.SharesPerToken = u8(8), rat(t, "1.0003771186767842")
	rhAAPL := evm("robinhood", "AAPL", "AAPL", chain.NetworkRobinhood, aaplRH)
	rhAAPL.SharesPerToken, rhAAPL.PendingSharesPerToken = rat(t, "1.000566"), rat(t, "1.002")
	rhAAPL.PendingAt = now.Add(-time.Minute)
	rhNVDA := evm("robinhood", "NVDA", "NVDA", chain.NetworkRobinhood, nvdaRH)
	rhNVDA.SharesPerToken, rhNVDA.PendingSharesPerToken = rat(t, "1.000776"), rat(t, "2")
	rhNVDA.PendingAt = now.Add(time.Hour)
	xEVM := evm("xstocks", "AAPL", "AAPLx", chain.NetworkEthereum, aaplXEVM)
	xEVM.WrapperAddress = aaplXWrap
	xSol := app.IssuerListing{
		Issuer: "xstocks", Ticker: "AAPL", Symbol: "AAPLx", Network: chain.NetworkSolana,
		TokenAddress: aaplXSolan, Decimals: u8(8), Tradable: true,
	}
	return []app.ListingSource{
		&source{
			issuer: "coinbase",
			batch:  app.ListingBatch{Issuer: "coinbase", Listings: []app.IssuerListing{aapl, tsla}},
		},
		&source{issuer: "robinhood", batch: app.ListingBatch{
			Issuer:   "robinhood",
			Listings: []app.IssuerListing{rhAAPL, rhNVDA},
		}},
		&source{
			issuer: "xstocks",
			batch:  app.ListingBatch{Issuer: "xstocks", Listings: []app.IssuerListing{xEVM, xSol}},
		},
	}
}

func TestListingSync_storesIssuerListingsAndEnablesOnlyTheVerifiedOnes(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	r := newListingRig(t, launchSources(t, now)...)
	r.verify.failing[tslaC] = true

	if changed := r.run(t); changed != 11 {
		t.Fatalf("first run changed %d, want 6 upserts and 5 verifications", changed)
	}
	want := []string{
		"coinbase AAPLc base " + aaplC + "  8 1 tradable=true enabled=true verified=true",
		"coinbase TSLAc base " + tslaC + "  8 1.0003771186767842 tradable=true enabled=false verified=false",
		"robinhood AAPL robinhood " + aaplRH + "  18 1.002 tradable=true enabled=true verified=true",
		"robinhood NVDA robinhood " + nvdaRH + "  18 1.000776 tradable=true enabled=true verified=true",
		"xstocks AAPLx ethereum " + aaplXEVM + " " + aaplXWrap + " 18 1 tradable=true enabled=true verified=true",
		"xstocks AAPLx solana " + aaplXSolan + "  8 1 tradable=true enabled=true verified=true",
	}
	if got := r.rows(t); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	failed := r.logged(t, "market.listings.verify_failed")
	if len(failed) != 1 || !strings.HasPrefix(failed[0], "WARN coinbase TSLAc base ") ||
		!strings.Contains(failed[0], "symbol() differs") {
		t.Fatalf("verify_failed logs = %q", failed)
	}

	delete(r.verify.failing, tslaC)
	if changed := r.run(t); changed != 1 {
		t.Fatalf("second run changed %d, want only TSLAc verified", changed)
	}
	want[1] = "coinbase TSLAc base " + tslaC + "  8 1.0003771186767842 tradable=true enabled=true verified=true"
	if got := r.rows(t); !slices.Equal(got, want) {
		t.Fatalf("rows after the retry =\n%s", strings.Join(got, "\n"))
	}
}

func TestListingSync_haltsAListingTheSourceLostAndAlertsOnce(t *testing.T) {
	t.Parallel()
	ondo := &source{issuer: "ondo", batch: app.ListingBatch{Issuer: "ondo", Listings: []app.IssuerListing{
		func() app.IssuerListing {
			l := evm("ondo", "AAPL", "AAPLon", chain.NetworkEthereum, aaplOn)
			l.Decimals, l.SharesPerToken = u8(18), rat(t, "1.003376")
			return l
		}(),
	}}}
	r := newListingRig(t, ondo)
	r.run(t)
	enabled := "ondo AAPLon ethereum " + aaplOn + "  18 1.003376 tradable=true enabled=true verified=true"
	if got := r.rows(t); !slices.Equal(got, []string{enabled}) {
		t.Fatalf("rows = %q", got)
	}

	ondo.batch = app.ListingBatch{Issuer: "ondo", Missing: []string{"AAPLon"}}
	if changed := r.run(t); changed != 1 {
		t.Fatalf("halting run changed %d, want 1", changed)
	}
	if changed := r.run(t); changed != 0 {
		t.Fatalf("second halting run changed %d, want 0", changed)
	}
	halted := "ondo AAPLon ethereum " + aaplOn + "  18 1.003376 tradable=false enabled=false verified=true"
	if got := r.rows(t); !slices.Equal(got, []string{halted}) {
		t.Fatalf("rows = %q", got)
	}
	if got := r.logged(t, "market.listings.source_missing"); !slices.Equal(got,
		[]string{"WARN ondo AAPLon ethereum <nil>"}) {
		t.Fatalf("source_missing logs = %q, want one alert over two runs", got)
	}
}

func TestListingSync_keepsTheOtherSourcesWhenOneFails(t *testing.T) {
	t.Parallel()
	down := &source{issuer: "robinhood", err: errs.New(errs.CodeUpstreamUnavailable, "test.Listings")}
	bad := &source{issuer: "acme", batch: app.ListingBatch{Issuer: "acme", Listings: []app.IssuerListing{
		evm("acme", "AAPL", "AAPLa", chain.NetworkBase, aaplC),
	}}}
	r := newListingRig(t, launchSources(t, clock.Real{}.Now())[0], down, bad)
	changed, err := r.sync.Run(r.ctx(t))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || !strings.Contains(err.Error(), "market.ListingSync.apply") {
		t.Fatalf("Run error = %v, want the source and the store failure joined", err)
	}
	if changed != 4 || len(r.rows(t)) != 2 {
		t.Fatalf("changed %d, rows %q, want Coinbase stored and verified", changed, r.rows(t))
	}
}

func TestListingSync_verifyAllUnverifiesAListingThatNoLongerPasses(t *testing.T) {
	t.Parallel()
	r := newListingRig(t, launchSources(t, clock.Real{}.Now())[:1]...)
	r.run(t)
	r.verify.failing[aaplC] = true
	report, err := r.sync.Verify(r.ctx(t), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Verified) != 1 || report.Verified[0].Symbol != "TSLAc" || report.Verified[0].VerifiedDecimals != 8 ||
		len(report.Failed) != 1 || report.Failed[0].Symbol != "AAPLc" || report.Failed[0].Err == nil {
		t.Fatalf("report = %+v", report)
	}
	want := []string{
		"coinbase AAPLc base " + aaplC + "  8 1 tradable=true enabled=false verified=false",
		"coinbase TSLAc base " + tslaC + "  8 1.0003771186767842 tradable=true enabled=true verified=true",
	}
	if got := r.rows(t); !slices.Equal(got, want) {
		t.Fatalf("rows =\n%s", strings.Join(got, "\n"))
	}
}

func TestListingSync_stopsWhenTheContextEnds(t *testing.T) {
	t.Parallel()
	r := newListingRig(t, launchSources(t, clock.Real{}.Now())[:1]...)
	done, cancel := context.WithCancel(r.ctx(t))
	cancel()
	if _, err := r.sync.Run(done); err == nil {
		t.Fatal("Run with an ended context succeeded")
	}
	if _, err := r.sync.Verify(done, true); err == nil {
		t.Fatal("Verify with an ended context succeeded")
	}
	r.run(t)
	midway, stop := context.WithCancel(r.ctx(t))
	defer stop()
	r.verify.cancel = stop
	if _, err := r.sync.Verify(midway, true); err == nil {
		t.Fatal("Verify cancelled midway succeeded")
	}
}

func TestSetListingOverride_turnsAListingOnAndOffAndBackToTheIssuer(t *testing.T) {
	t.Parallel()
	r := newListingRig(t, launchSources(t, clock.Real{}.Now())[:1]...)
	r.verify.failing[tslaC] = true
	r.run(t)
	for _, tc := range []struct {
		token    string
		override domain.Override
		enabled  bool
	}{
		{aaplC, domain.OverrideOff, false},
		{aaplC, domain.OverrideOn, true},
		{aaplC, domain.OverrideAuto, true},
		{tslaC, domain.OverrideOn, false},
	} {
		id := r.id(t, tc.token)
		got, err := app.SetListingOverride(t.Context(), r.pool, mustUUID(t, id), tc.override)
		if err != nil || got.Enabled != tc.enabled || got.ID.String() != id || got.TokenAddress != tc.token ||
			got.Network != chain.NetworkBase || got.Issuer != "coinbase" {
			t.Fatalf("override %s %s = %+v, %v, want enabled=%t", tc.token, tc.override, got, err, tc.enabled)
		}
	}
	missing := testkit.NewIDs(1).NewV7()
	if _, err := app.SetListingOverride(t.Context(), r.pool, missing, domain.OverrideOn); errs.CodeOf(err) !=
		errs.CodeNotFound {
		t.Fatalf("unknown listing = %v, want not_found", err)
	}
	done, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.SetListingOverride(done, r.pool, missing, domain.OverrideOn); err == nil ||
		errs.CodeOf(err) == errs.CodeNotFound {
		t.Fatalf("ended context = %v, want a database error", err)
	}
}

func TestCatalogPoller_syncsListingsAfterTheProviders(t *testing.T) {
	t.Parallel()
	r := newListingRig(t, launchSources(t, clock.Real{}.Now())[:1]...)
	g := testkit.NewIDs(4970)
	p := app.NewCatalogPoller(r.uow, r.pool, g, r.clock, app.NewProviders(),
		&marketfake.MintFacts{}).WithListings(r.sync)
	report, err := p.Tick(r.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed != 4 || !slices.ContainsFunc(report.Attrs, func(a slog.Attr) bool {
		return a.Equal(slog.Int("listings", 4))
	}) {
		t.Fatalf("report = %+v, want 2 listings stored and verified", report)
	}
}

func TestListingSync_leavesAListingWhoseChainDecimalsDifferDisabled(t *testing.T) {
	t.Parallel()
	listing := evm("ondo", "AAPL", "AAPLon", chain.NetworkEthereum, "0x14c3abf95cb9c93a8b82c1cdcb76d72cb87b2d4c")
	listing.Decimals, listing.SharesPerToken = u8(18), rat(t, "1")
	r := newListingRig(
		t,
		&source{issuer: "ondo", batch: app.ListingBatch{Issuer: "ondo", Listings: []app.IssuerListing{listing}}},
	)
	r.verify.decimals = map[string]uint8{listing.TokenAddress: 8}
	r.run(t)
	if got := r.rows(
		t,
	); len(got) != 1 ||
		!strings.HasSuffix(got[0], " 18 1 tradable=true enabled=false verified=false") {
		t.Fatalf("rows = %q, want the issuer's 18 kept and the listing disabled", got)
	}
	failed := r.logged(t, "market.listings.verify_failed")
	if len(failed) != 1 || !strings.Contains(failed[0], "chain_decimals=8") ||
		!strings.Contains(failed[0], "issuer_decimals=18") {
		t.Fatalf("verify_failed logs = %q", failed)
	}
}

func TestListingSync_keepsAListingTheOverrideTurnedOffDisabled(t *testing.T) {
	t.Parallel()
	listing := evm("xstocks", "AAPL", "AAPLx", chain.NetworkEthereum, aaplXEVM)
	listing.Decimals, listing.SharesPerToken = u8(18), rat(t, "1")
	r := newListingRig(
		t,
		&source{issuer: "xstocks", batch: app.ListingBatch{Issuer: "xstocks", Listings: []app.IssuerListing{listing}}},
	)
	_, err := r.pool.Exec(
		t.Context(),
		`INSERT INTO asset_listings (id, issuer, symbol, network, chain_id, token_address,
		decimals, shares_per_token, venue, enabled, tradable_override, created_at)
		VALUES ($1, 'xstocks', 'AAPLx', 'ethereum', 1, $2, 18, 1, 'uniswap', false, false, $3)`,
		testkit.NewIDs(4990).NewV7(),
		aaplXEVM,
		r.clock.Now(),
	)
	if err != nil {
		t.Fatal(err)
	}
	r.run(t)
	r.run(t)
	if got := r.rows(t); len(got) != 1 || !strings.HasSuffix(got[0], "tradable=true enabled=false verified=true") {
		t.Fatalf("rows = %q, want it verified but kept off by its override", got)
	}
}
