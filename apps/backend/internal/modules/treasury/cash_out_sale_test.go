package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type saleRig struct {
	f          fixture
	alice, bob ids.UserID
	cabal      ids.CabalID
	job        uuid.UUID
	started    events.CashOutStarted
}

func newSaleRig(t *testing.T, invested uint64) *saleRig {
	t.Helper()
	f := newFixture(t)
	r := &saleRig{f: f, alice: f.user(t), bob: f.user(t), cabal: f.cabal(t)}
	cashOutFund(t, f, r.alice, r.cabal, 50_000_000)
	cashOutFund(t, f, r.bob, r.cabal, 50_000_000)
	if invested > 0 {
		if err := f.confirm(t, buy(r.cabal, f.ids.NewV7(), invested, 1_000, 0)); err != nil {
			t.Fatal(err)
		}
	}
	h := cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{pot: money.MicrosFromUint64(100_000_000)})
	ctx := observability.WithActor(f.ctx(), "user:"+r.alice.String())
	result, err := h.Handle(ctx, app.CashOut{CabalID: r.cabal, UserID: r.alice, All: true})
	if err != nil {
		t.Fatal(err)
	}
	r.job = result.ID
	r.started = events.CashOutStarted{
		V: 1, JobID: r.job, CabalID: r.cabal.UUID(), UserID: r.alice.UUID(), ShareUnits: 100,
		PayoutMicros: money.MicrosFromUint64(50_000_000), SellUSDC: money.MicrosFromUint64(invested),
	}
	return r
}

func (r *saleRig) deliver(t *testing.T, e events.Event) {
	t.Helper()
	if err := r.apply(e); err != nil {
		t.Fatal(err)
	}
}

func (r *saleRig) apply(e events.Event) error { return r.applyIn(r.f.ctx(), e) }

func (r *saleRig) applyIn(ctx context.Context, e events.Event) error {
	m := treasury.New(module.Deps{Config: r.f.cfg, Pool: r.f.pool, IDs: r.f.ids, Clock: r.f.clock})
	ctx = observability.WithActor(ctx, "system:treasury.cashout")
	return r.f.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		for _, c := range m.Consumers() {
			for _, h := range c.Handlers {
				if h.Type() != e.Type() {
					continue
				}
				if err := h.Apply(ctx, tx, e, confirmedAt()); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func (r *saleRig) sold(swap uuid.UUID, units, micros uint64, batch int) events.TradeConfirmed {
	e := sell(r.cabal, swap, units, micros)
	e.Source, e.SourceBatchSize = events.TradeSource{Kind: "cashout", ID: r.job}, batch
	return e
}

func (r *saleRig) unsold(swap uuid.UUID, units uint64, batch int) events.TradeFailed {
	return events.TradeFailed{
		V: 1, SwapID: swap, CabalID: r.cabal.UUID(), Source: events.TradeSource{Kind: "cashout", ID: r.job},
		SourceBatchSize: batch, Action: "sell", Symbol: "AAPLx", InMint: chain.SolanaAddress(aapl), InAmount: units,
		FailureCode: "jupiter_failed",
	}
}

type jobState struct {
	Status, Payout, Returned string
	Code                     *string
}

func (r *saleRig) state(t *testing.T) jobState {
	t.Helper()
	var s jobState
	if err := r.f.pool.QueryRow(t.Context(), `SELECT status, payout_micros::text, returned_units::text, result_code
		FROM cash_out_jobs WHERE id = $1`, r.job).Scan(&s.Status, &s.Payout, &s.Returned, &s.Code); err != nil {
		t.Fatal(err)
	}
	return s
}

func (r *saleRig) shares(t *testing.T, user ids.UserID) uint64 {
	t.Helper()
	units, err := newQueries(r.f).ShareUnits(t.Context(), r.cabal, user)
	if err != nil {
		t.Fatal(err)
	}
	return units.Uint64()
}

func (r *saleRig) want(t *testing.T, status, payout, returned string, code string, aliceShares uint64) {
	t.Helper()
	got := r.state(t)
	gotCode := ""
	if got.Code != nil {
		gotCode = *got.Code
	}
	if got.Status != status || got.Payout != payout || got.Returned != returned || gotCode != code {
		t.Fatalf("job = %+v code %q, want %s payout %s returned %s code %q", got, gotCode, status, payout, returned,
			code)
	}
	if shares := r.shares(t, r.alice); shares != aliceShares {
		t.Fatalf("alice holds %d shares, want %d", shares, aliceShares)
	}
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %#v", drift)
	}
}

func (r *saleRig) failures(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = 'cashout.failed'
		AND aggregate_id = $1`, r.job).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *saleRig) originalTxn(t *testing.T) string {
	t.Helper()
	var status string
	if err := r.f.pool.QueryRow(t.Context(), `SELECT status FROM user_txns WHERE transfer_id = $1`, r.job).
		Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestCashOutSale_sellsTheShortfallAndPaysTheWholeSlice(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.want(t, "selling", "50000000", "0", "", 0)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 400, 21_000_000, 2))
	r.want(t, "selling", "50000000", "0", "", 0)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 200, 10_000_000, 2))
	r.want(t, "paying", "50000000", "0", "", 0)
	if r.shares(t, r.bob) != 100 {
		t.Fatal("bob's shares moved")
	}
}

func TestCashOutSale_shortSalePaysWhatItRaisedAndReturnsTheUncoveredUnits(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 400, 15_000_000, 2))
	r.deliver(t, r.unsold(r.f.ids.NewV7(), 200, 2))
	r.want(t, "paying", "35000000", "30", "sale_short", 30)
	if r.originalTxn(t) != "pending" || r.failures(t) != 0 {
		t.Fatalf("original txn %s with %d failures, want it pending for the payout", r.originalTxn(t),
			r.failures(t))
	}
}

func TestCashOutSale_nothingRaisedAndNothingOnHandFailsAndReturnsEveryUnit(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.unsold(r.f.ids.NewV7(), 500, 1))
	r.want(t, "failed", "50000000", "100", "sale_short", 100)
	if r.originalTxn(t) != "failed" || r.failures(t) != 1 {
		t.Fatalf("original txn %s with %d failures, want it failed once", r.originalTxn(t),
			r.failures(t))
	}
}

func TestCashOutSale_aResultBeforeTheStartIsKeptAndSettlesOnStart(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 600, 31_000_000, 1))
	r.want(t, "started", "50000000", "0", "", 0)
	r.deliver(t, r.started)
	r.want(t, "paying", "50000000", "0", "", 0)
}

func TestCashOutSale_ignoresOtherSourcesLateResultsAndRedelivery(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	other := r.sold(r.f.ids.NewV7(), 10, 1_000_000, 1)
	other.Source.Kind = "proposal"
	r.deliver(t, other)
	failed := r.unsold(r.f.ids.NewV7(), 10, 1)
	failed.Source.Kind = "proposal"
	r.deliver(t, failed)
	r.deliver(t, r.started)
	last := r.sold(r.f.ids.NewV7(), 600, 31_000_000, 1)
	r.deliver(t, last)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 10, 1_000_000, 1))
	r.want(t, "paying", "50000000", "0", "", 0)
	if n := r.f.count(t, "cash_out_sells"); n != 1 {
		t.Fatalf("%d sells recorded, want only the cash out's one", n)
	}
}

func TestCashOutSale_aCoveredJobNeverSells(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 0)
	r.deliver(t, r.started)
	r.want(t, "started", "50000000", "0", "", 0)
}

func TestCashOutSale_aStoreFailureRollsTheSettlementBack(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		setup string
		code  errs.Code
	}{
		{
			"share units the ledger cannot hold", `UPDATE cash_out_jobs SET share_units = 99999999999999999999`,
			errs.CodeDecodeFailed,
		},
		{
			"an unpaid part above int64", `UPDATE cash_out_jobs SET slice_micros = 18446744073709551615`,
			errs.CodeInvalidInput,
		},
		{"the job update refused", `CREATE FUNCTION refuse_settle() RETURNS trigger LANGUAGE plpgsql AS
			$$BEGIN RAISE EXCEPTION 'refused'; END$$;
			CREATE TRIGGER refuse_settle BEFORE UPDATE ON cash_out_jobs FOR EACH ROW
			WHEN (NEW.status <> 'selling') EXECUTE FUNCTION refuse_settle()`, errs.CodeInternal},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newSaleRig(t, 80_000_000)
			r.deliver(t, r.started)
			if _, err := r.f.pool.Exec(t.Context(), c.setup); err != nil {
				t.Fatal(err)
			}
			err := r.apply(r.sold(r.f.ids.NewV7(), 600, 15_000_000, 1))
			if err == nil || (c.code != errs.CodeInternal && errs.CodeOf(err) != c.code) {
				t.Fatalf("err = %v, want %s", err, c.code)
			}
			if got := r.state(t); got.Status != "selling" || r.f.count(t, "cash_out_sells") != 0 {
				t.Fatalf("job %+v with %d sells, want the delivery rolled back", got, r.f.count(t, "cash_out_sells"))
			}
		})
	}
}

func TestCashOutSale_anUnknownJobIsNotFound(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.job = r.f.ids.NewV7()
	started := r.started
	started.JobID = r.job
	for _, e := range []events.Event{started, r.sold(r.f.ids.NewV7(), 1, 1, 1)} {
		if err := r.apply(e); errs.CodeOf(err) != errs.CodeNotFound {
			t.Fatalf("%s for an unknown job: err = %v, want not_found", e.Type(), err)
		}
	}
}

func (r *saleRig) noLegs() events.TradeBlocked {
	return events.TradeBlocked{
		V: 1, CabalID: r.cabal.UUID(), Source: events.TradeSource{Kind: "cashout", ID: r.job}, Action: "sell",
		Code: errs.CodeAssetUntradable,
	}
}

func TestCashOutSale_noLegsPaysTheCashOnHandAndReturnsTheRest(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.noLegs())
	r.want(t, "paying", "20000000", "60", "sale_short", 60)
	r.deliver(t, r.noLegs())
	r.deliver(t, r.started)
	r.want(t, "paying", "20000000", "60", "sale_short", 60)
	if r.originalTxn(t) != "pending" || r.failures(t) != 0 {
		t.Fatalf("original txn %s with %d failures, want it pending for the payout", r.originalTxn(t),
			r.failures(t))
	}
}

func TestCashOutSale_noLegsAndNothingOnHandFailsAndReturnsEveryUnitOnce(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.deliver(t, r.noLegs())
	r.deliver(t, r.started)
	r.deliver(t, r.noLegs())
	r.want(t, "failed", "50000000", "100", "sale_short", 100)
	if r.originalTxn(t) != "failed" || r.failures(t) != 1 {
		t.Fatalf("original txn %s with %d failures, want it failed once", r.originalTxn(t), r.failures(t))
	}
}

func TestCashOutSale_noLegsIgnoresProposalsAndBatchedBlocks(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	proposal := r.noLegs()
	proposal.Source.Kind = "proposal"
	r.deliver(t, proposal)
	batched := r.noLegs()
	batched.SourceBatchSize = 1
	r.deliver(t, batched)
	r.want(t, "selling", "50000000", "0", "", 0)
}
