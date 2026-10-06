package funding_test

import (
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	flow08Treasury chain.SolanaAddress = "7Moh9vxpdx8awiKHGpWLFze6sXVeZgBphdP2EopQ78UG"
	flow08Sender   chain.SolanaAddress = "6CBBbEVgz7TsMc9yuU3tekHwnjE6BiSwFb4afWDrc8gd"
	flow08XStock   chain.SolanaAddress = "BcgXM4PrgxrBzBgEm2pvh7uDzKJ3ykw3SVLvzQgqF6xp"
	flow08Unknown  chain.SolanaAddress = "7cKjFHECViDkTcXeKsCXNFWaFYpHkJmVyEttvdacrgb4"
	flow08USDCSig  chain.Signature     = "4cu1Tu8V1ZuQdTNo5qjiDtDfaFsoaKmRW6dXW8pVSsHMhPcV1wsCDzDLrCzTehfLfSTQSva26SS7uUUVB8nswqp9"
	flow08DustSig  chain.Signature     = "4ABXAAQc1LWqaGi5dMVZZEf12hykGXmHWU6zR3bkUruM6pnNaV78QHVRp2M53ojHEbvRpCWE2cKMZoQEugfoNi4h"
	flow08StockSig chain.Signature     = "3EWpx8usE7nk6a5xEYswYH2GQDcFJVtceYLWoptFi1dMZsyiWg4sShQrMKPce1vhq2ejTDSR8eE7JoJ8QwfuwnoC"
	flow08OtherSig chain.Signature     = "sfLpFuMu7roifBc8qhCc2tnFUKtyscedzXgdRgNnm22w6YvDmYogC9UkjgYWZ6Md5A6a215gfE5PGcb6CAPrA1k"
)

type flow08 struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	cabal   testkit.SeededCabal
	funding *funding.Module
}

func newFlow08(t *testing.T) *flow08 {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now())
	cabal := testkit.NewCabal(t, pool)
	exec(t, pool, `UPDATE treasury_wallets SET address = '`+string(flow08Treasury)+`'`)
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	deps := module.Deps{
		Config: config.Config{
			Privy: config.Privy{
				BaseURL: "http://privy.test", VerificationKey: fakes.PrivyVerificationKey(),
			},
			Solana:   config.Solana{RPCURL: srv.URL + "/rpc/", USDCMint: string(testkit.USDCMint)},
			Timeouts: config.Timeouts{Privy: time.Second, RPC: 5 * time.Second},
		},
		Clock: clk, IDs: ids.Real{}, Pool: pool, UoW: db.New(pool, testkit.NewIDs(80), clk),
	}
	f := &flow08{pool: pool, clock: clk, cabal: cabal, funding: funding.New(deps)}
	module.NewSet(f.funding, trading.New(deps))
	return f
}

func (f *flow08) deliver(t *testing.T, sig chain.Signature) error {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:poller.funding.treasury-reconcile")
	_, err := f.funding.DetectExternalDeposit().Handle(ctx, app.DetectExternalDeposit{
		Signature: sig, CabalID: f.cabal.ID, Treasury: flow08Treasury, Source: domain.SourceReconcile,
	})
	return err
}

func (f *flow08) only(t *testing.T, sig chain.Signature) depositRow {
	t.Helper()
	rows := externalDeposits(t, f.pool)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one for %s", rows, sig)
	}
	return rows[string(sig)]
}

func TestFlow08_DetectExternalDeposit_OK(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)

	first, again := f.deliver(t, flow08USDCSig), f.deliver(t, flow08USDCSig)

	if first != nil || again != nil {
		t.Fatalf("detect = %v then %v, want nil twice", first, again)
	}
	row := f.only(t, flow08USDCSig)
	if row.status != "detected" || row.source != "reconcile" || row.amount != "25000000" ||
		row.mint != string(testkit.USDCMint) {
		t.Fatalf("row = %+v, want 25 USDC read from chain", row)
	}
	if externalPauses(t, f.pool) != 1 || countEvents(t, f.pool, events.TypeCabalPaused) != 1 ||
		countEvents(t, f.pool, events.TypeCabalExternalDepositDetected) != 1 {
		t.Fatal("want one pause, one cabal.paused and one cabal.external_deposit_detected")
	}
	if e := detectedEvent(t, f.pool); e.Sender != flow08Sender || e.Amount != 25_000_000 {
		t.Fatalf("event = %+v, want the chain's sender and amount", e)
	}
}

func TestFlow08_DetectExternalDeposit_BounceFailed(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	f.chain.closed = true
	id := f.detected(t, flow08USDCSig)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if f.status(t, id) != "bounce_failed" || len(f.transfers.builds) != 0 {
		t.Fatalf("status = %s, builds = %d, want bounce_failed with no transfer", f.status(t, id),
			len(f.transfers.builds))
	}
	if externalPauses(t, f.pool) != 1 || countEvents(t, f.pool, events.TypeCabalExternalDepositBounced) != 0 ||
		f.ledgerRows(t) != 0 {
		t.Fatal("a failed bounce ended the pause, emitted a bounced event or wrote ledger rows")
	}
	open, err := f.funding.ExternalDeposits().UnresolvedExternalDeposits(t.Context())
	if err != nil || len(open) != 1 {
		t.Fatalf("unresolved = %+v, %v, want the failed bounce listed for admins", open, err)
	}
}

func TestFlow08_DetectExternalDeposit_Dust(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)
	if err := f.deliver(t, flow08DustSig); err != nil {
		t.Fatal(err)
	}
	if row := f.only(t, flow08DustSig); row.status != "ignored_dust" || row.amount != "500000" {
		t.Fatalf("row = %+v, want ignored_dust of 500000", row)
	}
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalExternalDepositDetected) != 0 {
		t.Fatal("dust paused the cabal or emitted an event")
	}
}

func TestFlow08_DetectExternalDeposit_UnknownAsset(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)
	if err := f.deliver(t, flow08OtherSig); err != nil {
		t.Fatal(err)
	}
	if row := f.only(t, flow08OtherSig); row.status != "ignored_unknown" || row.mint != string(flow08Unknown) {
		t.Fatalf("row = %+v, want ignored_unknown", row)
	}
	if externalPauses(t, f.pool) != 0 || countEvents(t, f.pool, events.TypeCabalExternalDepositDetected) != 0 {
		t.Fatal("an unknown mint paused the cabal or emitted an event")
	}
}

func TestDetectExternalDeposit_ListedStockWithNoLivePriceIsDetected(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)
	asset := ids.Real{}.NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'AAPLx', $2, 8, 'xstocks', 'equity', 'Apple', true, 'apple', now(), now())`,
		asset, string(flow08XStock)); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(t, flow08StockSig); err != nil {
		t.Fatal(err)
	}
	row := f.only(t, flow08StockSig)
	if row.status != "detected" || row.assetID == nil || *row.assetID != asset || externalPauses(t, f.pool) != 1 {
		t.Fatalf("row = %+v, want a detected AAPLx row and a pause", row)
	}
}

func TestDetectExternalDeposit_OwnTransfersNeverCreateARow(t *testing.T) {
	t.Parallel()
	for owner, record := range map[string]string{
		"treasury fund sweep": `INSERT INTO fund_transfers (id, user_id, cabal_id, amount_micros, from_address,
			to_address, status, signed_tx, tx_signature, last_valid_block_height, created_at, submitted_at)
			VALUES (gen_random_uuid(), gen_random_uuid(), $1, 25000000, 'member', 'treasury', 'submitted', '\x00',
			$2, 1, now(), now())`,
		"trading swap": `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address, action, symbol,
			in_mint, out_mint, in_amount, slippage_bps, status, tx_signature, created_at, updated_at)
			VALUES (gen_random_uuid(), 'cashout', gen_random_uuid(), $1, 'treasury', 'sell', 'AAPLx', 'stock',
			'usdc', 1, 50, 'submitted', $2, now(), now())`,
		"treasury cash-out payout": `INSERT INTO user_txns (id, user_id, cabal_id, kind, status, tx_signature, created_at)
			VALUES (gen_random_uuid(), gen_random_uuid(), $1, 'cash_out', 'pending', $2, now())`,
	} {
		t.Run(owner, func(t *testing.T) {
			t.Parallel()
			f := newFlow08(t)
			if _, err := f.pool.Exec(t.Context(), record, f.cabal.ID.UUID(), string(flow08USDCSig)); err != nil {
				t.Fatal(err)
			}
			if err := f.deliver(t, flow08USDCSig); err != nil {
				t.Fatal(err)
			}
			if len(externalDeposits(t, f.pool)) != 0 || externalPauses(t, f.pool) != 0 {
				t.Fatalf("%s created a row or a pause", owner)
			}
		})
	}
}

func TestDetectExternalDeposit_PausesWhatFundCashOutAndTradingRead(t *testing.T) {
	t.Parallel()
	f := newFlow08(t)
	if err := f.deliver(t, flow08USDCSig); err != nil {
		t.Fatal(err)
	}

	pause, err := f.funding.Pauses().IsPaused(t.Context(), f.cabal.ID)
	if err != nil || !pause.Paused || len(pause.Reasons) != 1 ||
		pause.Reasons[0] != funding.PauseReasonExternalDeposit {
		t.Fatalf("IsPaused = %+v, %v, want paused for external_deposit", pause, err)
	}
	set, err := f.funding.Pauses().PausedCabals(t.Context())
	if err != nil || len(set.Cabals[f.cabal.ID]) != 1 || set.Global {
		t.Fatalf("PausedCabals = %+v, %v, want this cabal paused", set, err)
	}
}

func TestBounce_ReturnAddressOverridesTheSender(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	exec(t, f.pool, `UPDATE external_deposits SET return_address = '`+string(withdrawTo)+`'`)

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	if f.transfers.builds[0].To != withdrawTo {
		t.Fatalf("to = %s, want the return address", f.transfers.builds[0].To)
	}
}

func TestBounce_OpsPauseOutlivesTheBounce(t *testing.T) {
	t.Parallel()
	f := newBounceFixture(t)
	id := f.detected(t, flow08USDCSig)
	if _, err := f.funding.PauseFromOps(t.Context(), &f.cabal.ID, "investigating"); err != nil {
		t.Fatal(err)
	}

	if err := f.bouncer.Start(bounceCtx(t), id); err != nil {
		t.Fatal(err)
	}
	pause, err := f.funding.Pauses().IsPaused(t.Context(), f.cabal.ID)
	if err != nil || !pause.Paused || countEvents(t, f.pool, events.TypeCabalResumed) != 0 {
		t.Fatalf("IsPaused = %+v, %v, want the ops pause to hold", pause, err)
	}
}
