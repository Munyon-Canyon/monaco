package funding_test

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type watchChain struct {
	transfers map[chain.Signature][]solana.Transfer
	calls     atomic.Int32
}

func (c *watchChain) InboundTransfers(
	_ context.Context, sig chain.Signature, _ chain.SolanaAddress,
) ([]solana.Transfer, error) {
	c.calls.Add(1)
	return c.transfers[sig], nil
}

type watchAssets map[chain.SolanaAddress]market.Asset

func (a watchAssets) AssetByMint(_ context.Context, mint market.Mint) (market.Asset, error) {
	asset, ok := a[mint.Address()]
	if !ok {
		return market.Asset{}, errs.New(errs.CodeAssetNotFound, "test.AssetByMint")
	}
	return asset, nil
}

type watchPrices map[market.AssetID]market.Price

func (p watchPrices) LatestPrices(context.Context) (map[market.AssetID]market.Price, error) {
	return p, nil
}

type watchWallets struct {
	pages [][]identityport.MemberWallet
	calls atomic.Int32
}

func (w *watchWallets) MemberWallet(context.Context, ids.UserID) (identityport.MemberWallet, error) {
	return identityport.MemberWallet{}, errs.New(errs.CodeNotFound, "test.MemberWallet")
}

func (w *watchWallets) MemberWallets(context.Context, ids.UserID, int) ([]identityport.MemberWallet, error) {
	i := int(w.calls.Add(1)) - 1
	if i >= len(w.pages) {
		return nil, nil
	}
	return w.pages[i], nil
}

type ownedSigs map[chain.Signature]bool

func (o ownedSigs) OwnsSignature(_ context.Context, sig chain.Signature) (bool, error) {
	return o[sig], nil
}

type watchEnv struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	hints  *pauseHints
	chain  *watchChain
	cabal  testkit.SeededCabal
	usdc   chain.SolanaAddress
	stock  market.Asset
	prices watchPrices
	owned  ownedSigs
	deps   app.DetectDeps
}

func randomAddress(t *testing.T) chain.SolanaAddress {
	t.Helper()
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	return chain.AddressOf(key)
}

func newWatchEnv(t *testing.T, wallets ...identityport.MemberWallet) *watchEnv {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC())
	uow := db.New(pool, testkit.NewIDs(40), clk)
	var assetID market.AssetID
	if err := assetID.UnmarshalText([]byte(ids.Real{}.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	stockMint, err := market.ParseMint(string(randomAddress(t)))
	if err != nil {
		t.Fatal(err)
	}
	env := &watchEnv{
		pool:   pool,
		clock:  clk,
		hints:  &pauseHints{},
		chain:  &watchChain{transfers: map[chain.Signature][]solana.Transfer{}},
		cabal:  testkit.NewCabal(t, pool),
		usdc:   randomAddress(t),
		prices: watchPrices{},
		owned:  ownedSigs{},
		stock: market.Asset{
			ID: assetID, Symbol: "AAPLx", Mint: stockMint, Decimals: 8,
			UIMultiplier: market.Multiplier{Num: 1, Den: 1},
		},
	}
	env.deps = app.DetectDeps{
		UoW: uow, Reads: pool, IDs: ids.Real{}, Clock: clk, Hints: env.hints, Chain: env.chain,
		Owners: []fundingport.SignatureOwner{env.owned, adapters.NewBounceSignatures(pool)},
		Assets: watchAssets{stockMint.Address(): env.stock}, Prices: env.prices,
		Wallets: &watchWallets{pages: [][]identityport.MemberWallet{wallets}}, USDC: env.usdc,
	}
	return env
}

func (e *watchEnv) inbound(t *testing.T, mint chain.Mint, units uint64, from chain.SolanaAddress) chain.Signature {
	t.Helper()
	sig := chain.Signature(randomAddress(t) + randomAddress(t))
	amount := money.NewBaseUnits(units, mint.Decimals)
	e.chain.transfers[sig] = []solana.Transfer{{
		Signature: sig, From: from, Mint: mint, Amount: amount, Net: amount, Fee: money.NewBaseUnits(0, mint.Decimals),
	}}
	return sig
}

func (e *watchEnv) setPrice(price *market.Price) {
	if price == nil {
		return
	}
	p := *price
	if p.ObservedAt.IsZero() {
		p.ObservedAt = e.clock.Now()
	}
	e.prices[e.stock.ID] = p
}

func (e *watchEnv) usdcMint() chain.Mint { return chain.Mint{Address: e.usdc, Decimals: 6} }

func (e *watchEnv) stockMint() chain.Mint {
	return chain.Mint{Address: e.stock.Mint.Address(), Decimals: e.stock.Decimals}
}

func (e *watchEnv) try(ctx context.Context, sig chain.Signature, src domain.DepositSource) (app.DetectResult, error) {
	return app.NewDetectExternalDepositHandler(e.deps).Handle(ctx, app.DetectExternalDeposit{
		Signature: sig, CabalID: e.cabal.ID, Treasury: e.cabal.TreasuryAddress, Source: src,
	})
}

func (e *watchEnv) handle(t *testing.T, sig chain.Signature, src domain.DepositSource) app.DetectResult {
	t.Helper()
	res, err := e.try(observability.WithActor(t.Context(), "system:webhook.privy"), sig, src)
	if err != nil {
		t.Fatalf("Handle: %v", err)
	}
	return res
}

type depositRow struct {
	status, source, mint, amount string
	assetID                      *uuid.UUID
	resolved                     bool
}

func externalDeposits(t *testing.T, pool *pgxpool.Pool) map[string]depositRow {
	t.Helper()
	rows, err := pool.Query(t.Context(), `SELECT signature, status, source, mint, amount::text, asset_id,
		resolved_at IS NOT NULL FROM external_deposits`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := map[string]depositRow{}
	for rows.Next() {
		var sig string
		var r depositRow
		if err := rows.Scan(&sig, &r.status, &r.source, &r.mint, &r.amount, &r.assetID, &r.resolved); err != nil {
			t.Fatal(err)
		}
		out[sig] = r
	}
	return out
}

func externalPauses(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_pauses
		WHERE reason = 'external_deposit' AND external_deposit_id IS NOT NULL AND resolved_at IS NULL`,
	).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func detectedEvent(t *testing.T, pool *pgxpool.Pool) events.CabalExternalDepositDetected {
	t.Helper()
	var payload []byte
	if err := pool.QueryRow(t.Context(), `SELECT payload FROM events WHERE type = $1`,
		events.TypeCabalExternalDepositDetected).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var e events.CabalExternalDepositDetected
	if err := json.Unmarshal(payload, &e); err != nil {
		t.Fatal(err)
	}
	return e
}

func TestDetectExternalDeposit_USDCFromMemberPausesTheCabal(t *testing.T) {
	t.Parallel()
	member := identityport.MemberWallet{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), Address: randomAddress(t)}
	env := newWatchEnv(t, member)
	sig := env.inbound(t, env.usdcMint(), 25_000_000, member.Address)

	res := env.handle(t, sig, domain.SourceWebhook)

	if !res.Recorded || res.Verdict != domain.VerdictDetected || res.Outcome() != "" {
		t.Fatalf("result = %+v, want a recorded detection with outcome ok", res)
	}
	row := externalDeposits(t, env.pool)[string(sig)]
	if row.status != "detected" || row.source != "webhook" || row.amount != "25000000" || row.resolved {
		t.Fatalf("row = %+v, want an open detected webhook row of 25000000", row)
	}
	assertPausedOnce(t, env)
	e := detectedEvent(t, env.pool)
	want := events.CabalExternalDepositDetected{
		V: 1, ExternalDepositID: res.ExternalDepositID, CabalID: env.cabal.ID.UUID(), Signature: sig,
		Sender: e.Sender, Mint: env.usdc, Amount: 25_000_000,
	}
	sender := member.UserID.UUID()
	want.SenderUserID = &sender
	if e.SenderUserID == nil || *e.SenderUserID != sender || e.AssetID != nil {
		t.Fatalf("event = %+v, want the member as sender and no asset", e)
	}
	e.SenderUserID = want.SenderUserID
	if e != want {
		t.Fatalf("event = %+v, want %+v", e, want)
	}
}

func assertPausedOnce(t *testing.T, env *watchEnv) {
	t.Helper()
	if externalPauses(t, env.pool) != 1 || countEvents(t, env.pool, events.TypeCabalPaused) != 1 {
		t.Fatal("want one open external_deposit pause and one cabal.paused")
	}
	if got := env.hints.all(); len(got) != 1 || got[0] != "cabal."+env.cabal.ID.String()+".pause_changed" {
		t.Fatalf("hints = %v, want one pause_changed", got)
	}
}

func TestDetectExternalDeposit_RedeliveryAndPollerRecordOneRow(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	sig := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))

	first := env.handle(t, sig, domain.SourceWebhook)
	again := env.handle(t, sig, domain.SourceWebhook)
	polled := env.handle(t, sig, domain.SourceReconcile)

	if !first.Recorded || again.Recorded || polled.Recorded {
		t.Fatalf("recorded = %t/%t/%t, want only the first", first.Recorded, again.Recorded, polled.Recorded)
	}
	if n := len(externalDeposits(t, env.pool)); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	if externalPauses(t, env.pool) != 1 || countEvents(t, env.pool, events.TypeCabalExternalDepositDetected) != 1 {
		t.Fatal("want one pause and one detection event")
	}
}

func TestDetectExternalDeposit_ConcurrentDeliveriesRecordOneRow(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	sig := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	var recorded atomic.Int32
	var g errgroup.Group
	for range 4 {
		g.Go(func() error {
			res, err := env.try(observability.WithActor(t.Context(), "system:test"), sig, domain.SourceWebhook)
			if res.Recorded {
				recorded.Add(1)
			}
			return err
		})
	}
	if err := g.Wait(); err != nil {
		t.Fatal(err)
	}
	if recorded.Load() != 1 || externalPauses(t, env.pool) != 1 ||
		countEvents(t, env.pool, events.TypeCabalExternalDepositDetected) != 1 {
		t.Fatalf("recorded = %d, want one row, one pause and one event", recorded.Load())
	}
}

func TestDetectExternalDeposit_OwnSignatureIsSkippedBeforeAnyChainRead(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	sig := env.inbound(t, env.usdcMint(), 50_000_000, randomAddress(t))
	env.owned[sig] = true

	res := env.handle(t, sig, domain.SourceReconcile)

	if res.Recorded || res.Verdict != domain.VerdictOwn || res.Outcome() != "" || env.chain.calls.Load() != 0 {
		t.Fatalf("result = %+v after %d chain reads, want own and no read", res, env.chain.calls.Load())
	}
	if len(externalDeposits(t, env.pool)) != 0 || externalPauses(t, env.pool) != 0 {
		t.Fatal("an own transfer created a row or a pause")
	}
}

func TestDetectExternalDeposit_BounceSignatureIsOwn(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	bounced := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.handle(t, bounced, domain.SourceWebhook)
	bounce := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	if _, err := env.pool.Exec(
		t.Context(),
		`UPDATE external_deposits SET bounce_signature = $1`,
		string(bounce),
	); err != nil {
		t.Fatal(err)
	}

	if res := env.handle(t, bounce, domain.SourceReconcile); res.Verdict != domain.VerdictOwn || res.Recorded {
		t.Fatalf("result = %+v, want own", res)
	}
}

func TestDetectExternalDeposit_IgnoredTransfersNeverPause(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		send    func(*testing.T, *watchEnv) chain.Signature
		status  string
		outcome errs.Code
	}{
		{"usdc dust", func(t *testing.T, e *watchEnv) chain.Signature {
			t.Helper()
			return e.inbound(t, e.usdcMint(), 999_999, randomAddress(t))
		}, "ignored_dust", errs.CodeDust},
		{"priced stock dust", func(t *testing.T, e *watchEnv) chain.Signature {
			t.Helper()
			e.prices[e.stock.ID] = market.Price{Micros: money.MicrosFromUint64(200_000_000), ObservedAt: e.clock.Now()}
			return e.inbound(t, e.stockMint(), 400_000, randomAddress(t))
		}, "ignored_dust", errs.CodeDust},
		{"unknown mint", func(t *testing.T, e *watchEnv) chain.Signature {
			t.Helper()
			return e.inbound(t, chain.Mint{Address: randomAddress(t), Decimals: 9}, 1_000_000_000_000, randomAddress(t))
		}, "ignored_unknown", errs.CodeUnknownAsset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newWatchEnv(t)
			sig := tc.send(t, env)

			res := env.handle(t, sig, domain.SourceWebhook)

			if !res.Recorded || res.Outcome() != tc.outcome {
				t.Fatalf("result = %+v, want outcome %s", res, tc.outcome)
			}
			if row := externalDeposits(t, env.pool)[string(sig)]; row.status != tc.status || !row.resolved {
				t.Fatalf("row = %+v, want resolved %s", row, tc.status)
			}
			if externalPauses(t, env.pool) != 0 || countEvents(t, env.pool, events.TypeCabalPaused) != 0 ||
				countEvents(t, env.pool, events.TypeCabalExternalDepositDetected) != 0 || len(env.hints.all()) != 0 {
				t.Fatal("an ignored transfer paused the cabal or emitted an event")
			}
		})
	}
}

func TestDetectExternalDeposit_StockPricing(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		price *market.Price
		units uint64
	}{
		{"no live price is not dust", nil, 1},
		{"stale price is not dust", &market.Price{Micros: money.MicrosFromUint64(1), ObservedAt: time.Unix(0, 0)}, 1},
		{"worth exactly one dollar", &market.Price{Micros: money.MicrosFromUint64(200_000_000)}, 500_000},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := newWatchEnv(t)
			env.setPrice(tc.price)
			sig := env.inbound(t, env.stockMint(), tc.units, randomAddress(t))

			res := env.handle(t, sig, domain.SourceWebhook)

			if res.Verdict != domain.VerdictDetected || externalPauses(t, env.pool) != 1 {
				t.Fatalf("result = %+v, want a detection and a pause", res)
			}
			if e := detectedEvent(t, env.pool); e.AssetID == nil || *e.AssetID != env.stock.ID.UUID() {
				t.Fatalf("event asset = %v, want %s", e.AssetID, env.stock.ID)
			}
		})
	}
}

func TestDetectExternalDeposit_SecondDetectionAddsAPauseRowButNoSecondPausedEvent(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	env.handle(t, env.inbound(t, env.usdcMint(), 2_000_000, randomAddress(t)), domain.SourceWebhook)
	env.handle(t, env.inbound(t, env.usdcMint(), 3_000_000, randomAddress(t)), domain.SourceReconcile)

	if externalPauses(t, env.pool) != 2 || countEvents(t, env.pool, events.TypeCabalPaused) != 1 ||
		countEvents(t, env.pool, events.TypeCabalExternalDepositDetected) != 2 {
		t.Fatal("want two pause rows, one cabal.paused and two detections")
	}
	paused, err := adapters.NewPauses(env.pool).IsPaused(t.Context(), env.cabal.ID)
	if err != nil || !paused.Paused {
		t.Fatalf("IsPaused = %+v, %v, want paused", paused, err)
	}
}

func TestDetectExternalDeposit_NoInboundTransferRecordsNothing(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	if res := env.handle(t, chain.Signature(randomAddress(t)), domain.SourceReconcile); res.Recorded {
		t.Fatalf("result = %+v, want nothing recorded", res)
	}
	if len(externalDeposits(t, env.pool)) != 0 {
		t.Fatal("a signature with no inbound transfer created a row")
	}
}

type racingWallets struct {
	watchWallets
	race func()
}

func (w *racingWallets) MemberWallets(
	ctx context.Context, after ids.UserID, n int,
) ([]identityport.MemberWallet, error) {
	w.race()
	return w.watchWallets.MemberWallets(ctx, after, n)
}

func TestDetectExternalDeposit_ADeliveryThatLosesTheInsertRaceWritesNoPause(t *testing.T) {
	t.Parallel()
	env := newWatchEnv(t)
	sig := env.inbound(t, env.usdcMint(), 5_000_000, randomAddress(t))
	env.deps.Wallets = &racingWallets{race: func() {
		if _, err := env.pool.Exec(t.Context(), `INSERT INTO external_deposits
			(id, signature, cabal_id, sender, mint, amount, source, status, detected_at)
			VALUES (gen_random_uuid(), $1, $2, 'racer', 'mint', 1, 'reconcile', 'detected', now())`,
			string(sig), env.cabal.ID.UUID()); err != nil {
			t.Error(err)
		}
	}}
	res, err := env.try(t.Context(), sig, domain.SourceWebhook)
	if err != nil || res.Recorded {
		t.Fatalf("Handle = %+v, %v, want an unrecorded result and no error", res, err)
	}
	rows := externalDeposits(t, env.pool)
	if len(rows) != 1 || rows[string(sig)].source != "reconcile" {
		t.Fatalf("rows = %+v, want only the racer's row", rows)
	}
	if externalPauses(t, env.pool) != 0 || countEvents(t, env.pool, events.TypeCabalPaused) != 0 ||
		countEvents(t, env.pool, events.TypeCabalExternalDepositDetected) != 0 {
		t.Fatal("the losing delivery paused the cabal or emitted an event")
	}
}
