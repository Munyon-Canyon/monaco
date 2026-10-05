//go:build faultpoints

package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow07_FundCabal_CrashAfterSign(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	runs := 0
	testkit.CrashAt(t, faultpoint.AfterSign, func(ctx context.Context) error {
		runs++
		if runs == 1 {
			_, err := h.handler.Handle(
				auth.WithActor(ctx, auth.Actor{Kind: auth.ActorUser, ID: h.user.String()}),
				app.FundCabal{
					CabalID: h.cabal, UserID: h.user, Amount: money.MicrosFromUint64(60_000_000),
				},
			)
			return err
		}
		h.clock.Advance(app.FundSendWindow + time.Second)
		_, err := h.settler.Tick(withSystemActor(ctx))
		return err
	})
	var status, code string
	if err := h.pool.QueryRow(t.Context(), `SELECT status, fail_code FROM fund_transfers`).
		Scan(&status, &code); err != nil {
		t.Fatal(err)
	}
	inFlight, err := h.stubs.reads.InFlightMicros(h.ctx(), h.user)
	if status != "failed" || code != "fund_not_sent" || err != nil || !inFlight.IsZero() ||
		h.count(t, "user_txns") != 0 || len(h.stubs.ownedAtSend) != 0 {
		t.Fatalf("after a crash after signing: %s/%s, in flight %v (%v), want failed/fund_not_sent with nothing held",
			status, code, inFlight, err)
	}
}

func TestFlow07_FundCabal_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id := h.landed(t, "5000000")
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := h.settler.Tick(withSystemActor(ctx))
		return err
	})
	if status, units, _ := h.status(t, id); status != "settled" || units != "5000000" ||
		h.events(t, "cabal.funded") != 1 || h.count(t, "user_txns") != 1 || h.count(t, "cabal_txns") != 1 {
		t.Fatalf("after a crash before commit: %s with %s units, want one settled mint", status, units)
	}
}

func withSystemActor(ctx context.Context) context.Context {
	return auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: "poller.treasury.fund-transfers"})
}
