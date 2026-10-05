package treasury_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seededSig(n int) chain.Signature { return chain.Signature(fmt.Sprintf("seeded-signature-%d", n)) }

type payoutChain struct {
	mu       sync.Mutex
	readings map[chain.Signature][]domain.PayoutReading
	err      error
	onRead   func()
}

func (c *payoutChain) PayoutStatus(_ context.Context, sig chain.Signature, _ uint64) (domain.PayoutReading, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return domain.PayoutReading{}, c.err
	}
	if c.onRead != nil {
		c.onRead()
	}
	queue := c.readings[sig]
	if len(queue) == 0 {
		return domain.PayoutReading{State: domain.PayoutProcessing}, nil
	}
	if len(queue) > 1 {
		c.readings[sig] = queue[1:]
	}
	return queue[0], nil
}

func (c *payoutChain) set(sig chain.Signature, readings ...domain.PayoutReading) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.readings[sig] = readings
}

func landed() domain.PayoutReading { return domain.PayoutReading{State: domain.PayoutFinalized} }

func refused() domain.PayoutReading {
	return domain.PayoutReading{State: domain.PayoutFinalized, Failed: true}
}

func lapsed() domain.PayoutReading {
	return domain.PayoutReading{State: domain.PayoutNotFound, Expired: true}
}

func missing() domain.PayoutReading { return domain.PayoutReading{State: domain.PayoutNotFound} }

func processing() domain.PayoutReading { return domain.PayoutReading{State: domain.PayoutProcessing} }

type payoutHints struct {
	mu   sync.Mutex
	keys []string
}

func (h *payoutHints) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

type autoClock struct{ *testkit.Clock }

func (c autoClock) After(d time.Duration) <-chan time.Time {
	c.Advance(d)
	ch := make(chan time.Time, 1)
	ch <- c.Now()
	return ch
}

type payoutRig struct {
	f          fixture
	alice, bob ids.UserID
	cabal      ids.CabalID
	job        uuid.UUID
	chain      *payoutChain
	hints      *payoutHints
	transfers  *payoutTransfers
	wallets    payoutWallets
}

func newPayoutRig(t *testing.T) *payoutRig {
	t.Helper()
	f := newFixture(t)
	r := &payoutRig{f: f, alice: f.user(t), bob: f.user(t), cabal: f.cabal(t)}
	cashOutFund(t, f, r.alice, r.cabal, 50_000_000)
	cashOutFund(t, f, r.bob, r.cabal, 50_000_000)
	h := cashOutHandlerWith(f, cashOutPauses{}, cashOutValues{pot: money.MicrosFromUint64(100_000_000)})
	result, err := h.Handle(observability.WithActor(f.ctx(), "user:"+r.alice.String()),
		app.CashOut{CabalID: r.cabal, UserID: r.alice, All: true})
	if err != nil {
		t.Fatal(err)
	}
	r.job = result.ID
	return r.stubs()
}

func (r *payoutRig) stubs() *payoutRig {
	r.chain = &payoutChain{readings: map[chain.Signature][]domain.PayoutReading{}}
	r.hints = &payoutHints{}
	r.transfers = &payoutTransfers{}
	return r
}

func (r *payoutRig) payouts() *app.CashOutPayouts {
	return app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: autoClock{r.f.clock},
		Chain: r.chain, USDC: chain.Mint{Address: usdcMint, Decimals: 6}, Hints: r.hints,
		Transfers: func() (app.PayoutTransfers, error) { return r.transfers, nil }, Wallets: r.wallets,
	})
}

func (r *payoutRig) advance(t *testing.T, wait time.Duration) error {
	t.Helper()
	return r.payouts().Advance(observability.WithActor(r.f.ctx(), "system:treasury.cashout_payout"), r.job, wait, nil)
}

func (r *payoutRig) mustAdvance(t *testing.T) {
	t.Helper()
	if err := r.advance(t, 0); err != nil {
		t.Fatal(err)
	}
}

func (r *payoutRig) seed(t *testing.T, attempt int, status domain.PayoutStatus) {
	t.Helper()
	if _, err := r.f.pool.Exec(t.Context(),
		`UPDATE cash_out_jobs SET status = 'paying' WHERE id = $1 AND status = 'started'`, r.job); err != nil {
		t.Fatal(err)
	}
	if _, err := r.f.pool.Exec(t.Context(), `INSERT INTO cash_out_payouts
		(job_id, attempt, signature, signed_tx, last_valid_block_height, status, created_at)
		VALUES ($1, $2, $3, '\x01', 100, $4, $5)`,
		r.job, attempt, seededSig(attempt), string(status), r.f.clock.Now()); err != nil {
		t.Fatal(err)
	}
}

func (r *payoutRig) attempts(t *testing.T) []string {
	t.Helper()
	var out []string
	if err := r.f.pool.QueryRow(t.Context(), `SELECT coalesce(array_agg(attempt || ':' || status ORDER BY attempt),
		'{}') FROM cash_out_payouts WHERE job_id = $1`, r.job).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *payoutRig) wantJob(t *testing.T, status, code string) {
	t.Helper()
	var got, gotCode string
	if err := r.f.pool.QueryRow(t.Context(), `SELECT status, coalesce(result_code, '') FROM cash_out_jobs
		WHERE id = $1`, r.job).Scan(&got, &gotCode); err != nil {
		t.Fatal(err)
	}
	if got != status || gotCode != code {
		t.Fatalf("job = %s code %q, want %s code %q", got, gotCode, status, code)
	}
}

func (r *payoutRig) shares(t *testing.T, user ids.UserID) uint64 {
	t.Helper()
	units, err := newQueries(r.f).ShareUnits(t.Context(), r.cabal, user)
	if err != nil {
		t.Fatal(err)
	}
	return units.Uint64()
}

func (r *payoutRig) scalar(t *testing.T, query string, args ...any) string {
	t.Helper()
	var out string
	if err := r.f.pool.QueryRow(t.Context(), query, args...).Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *payoutRig) treasuryUSDC(t *testing.T) string {
	t.Helper()
	return r.scalar(t, `SELECT units::text FROM cabal_positions WHERE cabal_id = $1 AND asset = $2`,
		r.cabal.UUID(), usdcMint)
}

func (r *payoutRig) events(t *testing.T, typ events.Type) string {
	t.Helper()
	return r.scalar(t, `SELECT count(*)::text FROM events WHERE type = $1 AND aggregate_id = $2`, string(typ), r.job)
}

func (r *payoutRig) transferStatuses(t *testing.T) string {
	t.Helper()
	return r.scalar(t, `SELECT 'cabal=' || coalesce((SELECT string_agg(status, ',') FROM cabal_txns
		WHERE transfer_id = $1), '') || ' user=' || coalesce((SELECT string_agg(status, ',') FROM user_txns
		WHERE transfer_id = $1), '')`, r.job)
}

func (r *payoutRig) noDrift(t *testing.T) {
	t.Helper()
	if drift := r.f.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %#v", drift)
	}
}

func TestCashOutPayout_aLandedPayoutSettlesBothLedgersOnce(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), landed())
	r.mustAdvance(t)
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
	if got := r.attempts(t); !slices.Equal(got, []string{"1:confirmed"}) {
		t.Fatalf("attempts = %v", got)
	}
	if r.treasuryUSDC(t) != "50000000" || r.transferStatuses(t) != "cabal=settled user=settled" {
		t.Fatalf("treasury %s USDC, headers %s", r.treasuryUSDC(t), r.transferStatuses(t))
	}
	if r.events(t, events.TypeCashOutCompleted) != "1" || r.shares(t, r.alice) != 0 {
		t.Fatalf("completed events %s, alice shares %d", r.events(t, events.TypeCashOutCompleted), r.shares(t, r.alice))
	}
	if !slices.Contains(r.hints.keys, events.UserCashOutChangedHint(r.alice)) ||
		!slices.Contains(r.hints.keys, events.UserBalanceChangedHint(r.alice)) {
		t.Fatalf("hints = %v", r.hints.keys)
	}
	r.noDrift(t)
}

func TestCashOutPayout_givesUpOnceEveryAttemptLapsedAndReturnsEveryUnit(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutExpired)
	r.seed(t, 2, domain.PayoutExpired)
	r.seed(t, 3, domain.PayoutBroadcast)
	r.chain.set(seededSig(3), lapsed())
	r.mustAdvance(t)
	r.wantJob(t, "failed", string(errs.CodePayoutFailed))
	if got := r.attempts(t); !slices.Equal(got, []string{"1:expired", "2:expired", "3:expired"}) {
		t.Fatalf("attempts = %v", got)
	}
	if r.shares(t, r.alice) != 100 || r.treasuryUSDC(t) != "100000000" ||
		r.transferStatuses(t) != "cabal= user=failed" {
		t.Fatalf("alice shares %d, treasury %s, headers %s", r.shares(t, r.alice), r.treasuryUSDC(t),
			r.transferStatuses(t))
	}
	r.mustAdvance(t)
	if r.events(t, events.TypeCashOutFailed) != "1" || r.shares(t, r.alice) != 100 {
		t.Fatal("a second run returned the units twice")
	}
	r.noDrift(t)
}

func TestCashOutPayout_aLapsedAttemptIsClosedBeforeAnotherIsSigned(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), lapsed())
	r.mustAdvance(t)
	r.wantJob(t, "paying", "")
	if got := r.attempts(t); got[0] != "1:expired" {
		t.Fatalf("attempts = %v", got)
	}
}

func TestCashOutPayout_aPayoutRefusedOnChainFailsTheJob(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), refused())
	r.mustAdvance(t)
	r.wantJob(t, "failed", string(errs.CodePayoutFailed))
	if got := r.attempts(t); !slices.Equal(got, []string{"1:failed"}) || r.shares(t, r.alice) != 100 {
		t.Fatalf("attempts = %v, alice shares %d", got, r.shares(t, r.alice))
	}
	r.noDrift(t)
}

func TestCashOutPayout_aPayoutStillInFlightStaysPaying(t *testing.T) {
	t.Parallel()
	for name, reading := range map[string]domain.PayoutReading{"processing": processing(), "missing": missing()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			r.seed(t, 1, domain.PayoutBroadcast)
			r.chain.set(seededSig(1), reading)
			r.mustAdvance(t)
			r.wantJob(t, "paying", "")
			if got := r.attempts(t); !slices.Equal(got, []string{"1:broadcast"}) {
				t.Fatalf("attempts = %v", got)
			}
		})
	}
}

func TestCashOutPayout_waitsForTheChainUpToTheDeadline(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), processing(), landed())
	if err := r.advance(t, app.CashOutPayoutWait); err != nil {
		t.Fatal(err)
	}
	r.wantJob(t, "completed", "")
}

func TestCashOutPayout_aShortSaleIsPaidAsPartial(t *testing.T) {
	t.Parallel()
	s := newSaleRig(t, 80_000_000)
	r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
	s.deliver(t, s.started)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 15_000_000, 2))
	s.deliver(t, s.unsold(s.f.ids.NewV7(), 200, 2))
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), landed())
	r.mustAdvance(t)
	r.wantJob(t, "partial", "sale_short")
	if r.shares(t, s.alice) != 30 {
		t.Fatalf("alice holds %d shares, want the 30 the sale did not cover", r.shares(t, s.alice))
	}
	if r.events(t, events.TypeCashOutPartial) != "1" || r.events(t, events.TypeCashOutCompleted) != "0" {
		t.Fatalf("partial events %s, completed events %s, want 1 and 0",
			r.events(t, events.TypeCashOutPartial), r.events(t, events.TypeCashOutCompleted))
	}
	got := r.scalar(t, `SELECT payload->>'share_units_burned' || '/' || (payload->>'share_units_returned') || '/' ||
		(payload->>'payout_micros') FROM events WHERE type = 'cashout.partial' AND aggregate_id = $1`, r.job)
	if got != "70/30/"+r.scalar(t, `SELECT payout_micros::text FROM cash_out_jobs WHERE id = $1`, r.job) {
		t.Fatalf("cashout.partial burned/returned/payout = %s", got)
	}
	r.noDrift(t)
}

func TestCashOutPayout_aFailedPayoutAfterAShortSaleReturnsTheRestOfTheUnits(t *testing.T) {
	t.Parallel()
	s := newSaleRig(t, 80_000_000)
	r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
	s.deliver(t, s.started)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 15_000_000, 2))
	s.deliver(t, s.unsold(s.f.ids.NewV7(), 200, 2))
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), refused())
	r.mustAdvance(t)
	r.wantJob(t, "failed", "payout_failed")
	if r.shares(t, s.alice) != 100 {
		t.Fatalf("alice holds %d shares, want every unit back", r.shares(t, s.alice))
	}
	r.noDrift(t)
}
