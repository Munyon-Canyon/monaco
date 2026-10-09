package treasury_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestFundOutflows_holdCreatedAndSubmittedFundsAgainstTheBalance(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	for i, status := range fundStatuses() {
		f.fundTransferIn(t, user, status, int64(i+1)*1_000_000)
	}
	outflows := treasury.New(module.Deps{Pool: f.pool}).FundOutflows()
	got, err := outflows.InFlightMicros(f.ctx(), user)
	if err != nil || got != money.MicrosFromUint64(3_000_000) {
		t.Fatalf("InFlightMicros = %v, %v, want 3000000", got, err)
	}
	if _, err := outflows.InFlightMicros(canceled(f.ctx()), user); err == nil {
		t.Fatal("InFlightMicros on a canceled context = nil error")
	}
}

func TestFundOutflows_lastChangeIsTheUsersLatestMove(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	outflows := treasury.New(module.Deps{Pool: f.pool}).FundOutflows()
	assertLast := func(want time.Time) {
		t.Helper()
		if got, err := outflows.LastChange(f.ctx(), user); err != nil || !got.Equal(want) {
			t.Fatalf("LastChange = %v, %v, want %v", got, err, want)
		}
	}
	assertLast(time.Unix(0, 0))
	id := f.fundTransferIn(t, user, domain.FundCreated, 1_000_000)
	assertLast(f.clock.Now())
	for _, to := range []domain.FundStatus{domain.FundSubmitted, domain.FundLanded, domain.FundSettled} {
		f.clock.Advance(time.Second)
		if !f.moveFundTransfer(t, id, to) {
			t.Fatalf("move to %s changed no row", to)
		}
		assertLast(f.clock.Now())
	}
	f.clock.Advance(time.Second)
	failed := f.fundTransferIn(t, user, domain.FundCreated, 1_000_000)
	latest := f.clock.Now()
	f.clock.Advance(time.Second)
	if !f.moveFundTransfer(t, failed, domain.FundFailed) {
		t.Fatal("move to failed changed no row")
	}
	f.fundTransferIn(t, f.user(t), domain.FundCreated, 1_000_000)
	assertLast(latest)
	if _, err := outflows.LastChange(canceled(f.ctx()), user); err == nil {
		t.Fatal("LastChange on a canceled context = nil error")
	}
}

func TestFundOutflows_submittedIsTheUsersSubmittedAmountBySignature(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	want := map[chain.Signature]money.Micros{}
	for i, status := range fundStatuses() {
		id := f.fundTransferIn(t, user, status, int64(i+1)*1_000_000)
		if status == domain.FundSubmitted {
			want[fundSignature(id)] = money.MicrosFromUint64(uint64(i+1) * 1_000_000)
		}
	}
	f.fundTransferIn(t, f.user(t), domain.FundSubmitted, 9_000_000)
	outflows := treasury.New(module.Deps{Pool: f.pool}).FundOutflows()
	if got, err := outflows.Submitted(f.ctx(), user); err != nil || !maps.Equal(got, want) {
		t.Fatalf("Submitted = %v, %v, want %v", got, err, want)
	}
	if _, err := outflows.Submitted(canceled(f.ctx()), user); err == nil {
		t.Fatal("Submitted on a canceled context = nil error")
	}
	huge := f.fundTransferIn(t, user, domain.FundSubmitted, 1_000_000)
	if _, err := f.pool.Exec(f.ctx(), `UPDATE fund_transfers SET amount_micros = 99999999999999999999 WHERE id = $1`,
		huge); err != nil {
		t.Fatal(err)
	}
	if _, err := outflows.Submitted(f.ctx(), user); err == nil {
		t.Fatal("Submitted with an amount past uint64 = nil error")
	}
}

func canceled(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}
