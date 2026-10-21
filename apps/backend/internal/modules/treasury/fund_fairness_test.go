package treasury_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	stuckFunds    = 100
	fairnessTicks = 6
)

func (h *settleHarness) stickFunds(t *testing.T, n int) []uuid.UUID {
	t.Helper()
	waiting := map[uuid.UUID]bool{}
	stuck := make([]uuid.UUID, n)
	for i := range stuck {
		stuck[i] = h.fundTransferIn(t, h.user, domain.FundLanded, 5_000_000)
		waiting[h.transfer(t, stuck[i]).CabalID] = true
	}
	h.chain.potHook = func(_ context.Context, cabal ids.CabalID) error {
		if waiting[cabal.UUID()] {
			return errs.New(errs.CodePriceUnavailable, "stub")
		}
		return nil
	}
	return stuck
}

func (h *settleHarness) tickWithinBudget(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(h.systemCtx(), poller.TickBudget(adapters.FundPoller{Settler: h.settler}))
	defer cancel()
	if _, err := h.settler.Tick(ctx); err != nil && ctx.Err() == nil {
		t.Fatal(err)
	}
	h.clock.Advance(app.FundSettleInterval)
}

func (h *settleHarness) tickUntilSettled(t *testing.T, id uuid.UUID, within int) {
	t.Helper()
	for tick := 1; tick <= within; tick++ {
		h.tickWithinBudget(t)
		if status, _, _ := h.status(t, id); status == "settled" {
			return
		}
	}
	t.Fatalf("fund %s not settled after %d ticks", id, within)
}

func (h *settleHarness) assertStillWaiting(t *testing.T, stuck []uuid.UUID) {
	t.Helper()
	for _, id := range stuck {
		if status, units, _ := h.status(t, id); status != "landed" || units != "0" {
			t.Fatalf("stuck fund %s = %s/%s, want landed with no shares", id, status, units)
		}
	}
	if n := h.events(t, "cabal.funded"); n != 1 {
		t.Fatalf("cabal.funded events = %d, want only the healthy fund's", n)
	}
	if n := h.count(t, "user_txns"); n != 1 {
		t.Fatalf("user_txns = %d, want only the healthy fund's", n)
	}
}

func TestFundSettler_aHundredStuckFundsDoNotLockOutAHealthyOne(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	stuck := h.stickFunds(t, stuckFunds)
	h.clock.Advance(time.Minute)
	healthy := h.fundTransferIn(t, h.user, domain.FundLanded, 5_000_000)
	h.tickUntilSettled(t, healthy, fairnessTicks)
	h.assertStillWaiting(t, stuck)
}

func TestFundSettler_aFundThatLandsLaterGetsItsTurnAfterARestart(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	stuck := h.stickFunds(t, stuckFunds)
	h.clock.Advance(time.Minute)
	healthy, sig := h.submitted(t, 5_000_000)
	h.tickWithinBudget(t)
	h.tickWithinBudget(t)
	h.settler = h.newSettler(0)
	h.finalize(sig)
	h.tickUntilSettled(t, healthy, fairnessTicks)
	h.assertStillWaiting(t, stuck)
}

func TestFundSettler_everyVisitIsStampedSoRowsTakeTurns(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	stuck := h.stickFunds(t, 3)
	for _, id := range stuck {
		if at := h.lastAttempt(t, id); at != nil {
			t.Fatalf("fund %s attempted at %v before any tick", id, at)
		}
	}
	h.tickWithinBudget(t)
	for _, id := range stuck {
		if at := h.lastAttempt(t, id); at == nil {
			t.Fatalf("fund %s has no last_attempt_at after a tick", id)
		}
	}
}

func (h *settleHarness) lastAttempt(t *testing.T, id uuid.UUID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := h.pool.QueryRow(t.Context(), `SELECT last_attempt_at FROM fund_transfers WHERE id = $1`, id).
		Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func TestFundSettler_aSlowFundIsCutOffAndTheRestStillSettle(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	slow := h.stickFunds(t, 1)
	h.clock.Advance(time.Minute)
	healthy := h.fundTransferIn(t, h.user, domain.FundLanded, 5_000_000)
	slowCabal := h.transfer(t, slow[0]).CabalID
	h.chain.potHook = func(ctx context.Context, cabal ids.CabalID) error {
		if cabal.UUID() != slowCabal {
			return nil
		}
		<-ctx.Done()
		return ctx.Err()
	}
	h.settler = h.newSettler(time.Second)
	for range fairnessTicks {
		if status, _, _ := h.status(t, healthy); status == "settled" {
			break
		}
		ctx, cancel := context.WithTimeout(h.systemCtx(), poller.TickBudget(adapters.FundPoller{Settler: h.settler}))
		_, _ = h.settler.Tick(ctx)
		cancel()
		h.clock.Advance(app.FundSettleInterval)
	}
	if status, units, _ := h.status(t, healthy); status != "settled" || units != "5000000" {
		t.Fatalf("healthy fund = %s/%s, want settled 5000000", status, units)
	}
	if status, _, _ := h.status(t, slow[0]); status != "landed" {
		t.Fatalf("slow fund = %s, want landed", status)
	}
}

type noExec struct{ sqlc.DBTX }

var errNoExec = errs.New(errs.CodeInternal, "test.noExec")

func (noExec) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, errNoExec
}

func TestFundSettler_aTickOutOfTimeStopsWithoutAnErrorAndKeepsTheRestInLine(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	stuck := h.stickFunds(t, 3)
	ctx, cancel := context.WithCancel(h.systemCtx())
	defer cancel()
	h.chain.potHook = func(ctx context.Context, _ ids.CabalID) error {
		cancel()
		return ctx.Err()
	}
	if _, err := h.settler.Tick(ctx); err != nil {
		t.Fatalf("Tick out of time = %v, want nil", err)
	}
	attempted := 0
	for _, id := range stuck {
		if h.lastAttempt(t, id) != nil {
			attempted++
		}
	}
	if attempted != 1 {
		t.Fatalf("funds stamped = %d, want only the one being worked", attempted)
	}
}

func TestFundSettler_aFundItCannotStampIsNotWorked(t *testing.T) {
	t.Parallel()
	t.Run("landed", func(t *testing.T) {
		t.Parallel()
		h := newSettleHarness(t)
		id := h.landed(t, "5000000")
		h.settler = h.newSettlerReading(noExec{h.pool}, 0)
		if err := h.tickErr(); !errors.Is(err, errNoExec) {
			t.Fatalf("Tick = %v, want %v", err, errNoExec)
		}
		if status, _, _ := h.status(t, id); status != "landed" {
			t.Fatalf("status = %s, want landed", status)
		}
	})
	t.Run("submitted", func(t *testing.T) {
		t.Parallel()
		h := newSettleHarness(t)
		id, sig := h.submitted(t, 5_000_000)
		h.finalize(sig)
		h.settler = h.newSettlerReading(noExec{h.pool}, 0)
		if err := h.tickErr(); !errors.Is(err, errNoExec) {
			t.Fatalf("Tick = %v, want %v", err, errNoExec)
		}
		if status, _, _ := h.status(t, id); status != "submitted" {
			t.Fatalf("status = %s, want submitted", status)
		}
	})
}
