package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type raceRig struct {
	f       fixture
	cabal   ids.CabalID
	a, b    ids.UserID
	jobA    app.CashOutResult
	started events.CashOutStarted
}

func newRaceRig(t *testing.T) *raceRig {
	t.Helper()
	f := newFixture(t)
	r := &raceRig{f: f, cabal: f.cabal(t), a: f.user(t), b: f.user(t)}
	c := f.user(t)
	for _, u := range []ids.UserID{r.a, r.b, c} {
		cashOutFund(t, f, u, r.cabal, 100_000_000)
	}
	buy, err := f.swap(r.cabal, 300_000_000, 3_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(buy); err != nil {
		t.Fatal(err)
	}
	r.jobA = raceCashOutAll(t, f, r.cabal, r.a, 300_000_000)
	r.started = events.CashOutStarted{
		V: 1, JobID: r.jobA.ID, CabalID: r.cabal.UUID(), UserID: r.a.UUID(), ShareUnits: r.jobA.ShareUnits.Uint64(),
		PayoutMicros: r.jobA.PayoutMicros, SellUSDC: money.MicrosFromUint64(100_000_000), Cause: "member",
	}
	return r
}

func raceCashOutAll(t *testing.T, f fixture, cabal ids.CabalID, user ids.UserID, pot uint64) app.CashOutResult {
	t.Helper()
	h := cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{pot: money.MicrosFromUint64(pot)})
	result, err := h.Handle(
		observability.WithActor(f.ctx(), "user:"+user.String()),
		app.CashOut{CabalID: cabal, UserID: user, All: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func (r *raceRig) bookProceeds(t *testing.T) {
	t.Helper()
	sold, err := domain.NewCabalTxn(domain.CabalTxnHeader{
		ID: r.f.ids.NewV7(), CabalID: r.cabal, Kind: domain.CabalSwap, Status: domain.TxnSettled,
		SwapID: r.f.ids.NewV7(),
	}, []domain.CabalEntry{
		{Account: domain.CabalTreasury, Asset: aapl, Amount: amount(-1_010_000)},
		{Account: domain.CabalVenue, Asset: aapl, Amount: amount(1_010_000)},
		{Account: domain.CabalTreasury, Asset: r.f.usdc(), Amount: amount(101_000_000)},
		{Account: domain.CabalVenue, Asset: r.f.usdc(), Amount: amount(-101_000_000)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := r.f.postCabal(sold); err != nil {
		t.Fatal(err)
	}
}

func (r *raceRig) tallyProceeds(t *testing.T) {
	t.Helper()
	sales := app.NewCashOutSales(r.f.ledger, r.f.ids)
	err := r.f.do(func(ctx context.Context, tx db.Tx) error {
		if err := sales.Started(ctx, tx, r.started, r.f.clock.Now()); err != nil {
			return err
		}
		return sales.Result(ctx, tx, app.SaleResult{
			JobID: r.jobA.ID, SwapID: r.f.ids.NewV7(), CabalID: r.cabal, BatchSize: 1, Confirmed: true,
			USDCOut: money.MicrosFromUint64(101_000_000),
		}, r.f.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (r *raceRig) wantSecondJob(t *testing.T, job app.CashOutResult) {
	t.Helper()
	var cash, payout, sell string
	if err := r.f.pool.QueryRow(t.Context(), `SELECT units::text FROM cabal_positions
		WHERE cabal_id = $1 AND asset = $2`, r.cabal.UUID(), string(r.f.usdc())).Scan(&cash); err != nil {
		t.Fatal(err)
	}
	if err := r.f.pool.QueryRow(t.Context(), `SELECT payout_micros::text, sell_usdc_micros::text
		FROM cash_out_jobs WHERE id = $1`, job.ID).Scan(&payout, &sell); err != nil {
		t.Fatal(err)
	}
	if cash != "101000000" || payout != "100000000" || sell != "99000000" {
		t.Fatalf("treasury USDC=%s, second payout=%s, second sell=%s; want 101000000, 100000000, 99000000",
			cash, payout, sell)
	}
}

func TestCashOut_secondCashOutDoesNotSpendTheFirstCashOutsSaleProceedsWhenTheLedgerIsFirst(t *testing.T) {
	t.Parallel()
	r := newRaceRig(t)
	r.bookProceeds(t)
	second := raceCashOutAll(t, r.f, r.cabal, r.b, 200_000_000)
	r.tallyProceeds(t)
	r.wantSecondJob(t, second)
}

func TestCashOut_secondCashOutDoesNotSpendTheFirstCashOutsSaleProceedsWhenTheTallyIsFirst(t *testing.T) {
	t.Parallel()
	r := newRaceRig(t)
	r.bookProceeds(t)
	r.tallyProceeds(t)
	second := raceCashOutAll(t, r.f, r.cabal, r.b, 200_000_000)
	r.wantSecondJob(t, second)
}
