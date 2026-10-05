package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
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

func canceled(ctx context.Context) context.Context {
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	return ctx
}
