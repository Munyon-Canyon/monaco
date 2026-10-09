package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
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

func canceled(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}
