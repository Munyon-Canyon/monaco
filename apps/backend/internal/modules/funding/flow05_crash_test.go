//go:build faultpoints

package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestFlow05_CreditDeposit_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	cmd := depositCommand(t, user, 1)
	h := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(91), testkit.NewClock(cmd.CreditedAt)), &hints{})
	testkit.CrashAt(t, faultpoint.BeforeCommit, func(ctx context.Context) error {
		_, err := h.Handle(observability.WithActor(ctx, "system:poller.funding.deposits"), cmd)
		return err
	})
	var deposits, events int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), (SELECT count(*) FROM events) FROM deposits`,
	).Scan(&deposits, &events); err != nil {
		t.Fatal(err)
	}
	if deposits != 1 || events != 1 {
		t.Fatalf("after crash and retry deposits/events = %d/%d, want 1/1", deposits, events)
	}
}

func depositCommand(t *testing.T, user testkit.SeededUser, micros uint64) app.CreditDeposit {
	t.Helper()
	now := clock.Real{}.Now().UTC()
	sig := chain.Signature(depositSignature)
	return app.CreditDeposit{
		ID: testkit.NewIDs(90).NewV7(), UserID: user.ID, WalletAddress: user.Address,
		TxSignature: sig, CursorSignature: sig, Amount: money.MicrosFromUint64(micros),
		Slot: 42, BlockTime: now, CreditedAt: now,
	}
}
