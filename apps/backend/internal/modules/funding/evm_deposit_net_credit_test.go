package funding_test

import (
	"hash/crc32"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	firstBaseForwardInMicros       = 999_250
	firstBaseForwardNetOfFeeMicros = 835_383
)

func netExecution(id string, state mc.ActionState, micros uint64) mc.Execution {
	ex := execution(id, state)
	ex.AmountMicros, ex.Signature = money.MicrosFromUint64(micros), ""
	return ex
}

func (f *forwardFixture) deposit(t *testing.T, sig string, micros uint64, at time.Time) {
	t.Helper()
	id := testkit.NewIDs(uint64(crc32.ChecksumIEEE([]byte(sig)))).NewV7()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO deposits
		(id, user_id, wallet_address, tx_signature, amount_micros, slot, credited_at)
		VALUES ($1, $2, $3, $4, $5, 1, $6)`, id, f.user.ID.UUID(), string(f.user.Address), sig, micros, at,
	); err != nil {
		t.Fatal(err)
	}
}

func (f *forwardFixture) forwardState(t *testing.T, execution string) (string, string, string) {
	t.Helper()
	var status, amount string
	var signature *string
	if err := f.pool.QueryRow(t.Context(), `SELECT status, amount_micros::text, solana_signature
		FROM evm_deposit_forwards WHERE privy_execution_id = $1`, execution).
		Scan(&status, &amount, &signature); err != nil {
		t.Fatal(err)
	}
	if signature == nil {
		return status, amount, ""
	}
	return status, amount, *signature
}

func TestEVMDepositForward_SucceededExecutionArrivesOnTheNetCreditThatLandedFirst(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	created := f.clock.Now()
	f.tick(t, netExecution("a", mc.ActionPending, firstBaseForwardInMicros))
	if status, amount, _ := f.forwardState(t, "a"); status != "pending" || amount != "999250" {
		t.Fatalf("pending forward = %s %s, want pending 999250", status, amount)
	}
	f.deposit(t, "before-the-forward", firstBaseForwardNetOfFeeMicros, created.Add(-time.Minute))
	f.deposit(t, "the-pending-amount", firstBaseForwardInMicros, created.Add(30*time.Second))
	f.deposit(t, "net-credit", firstBaseForwardNetOfFeeMicros, created.Add(time.Minute))
	f.clock.Advance(2 * time.Minute)
	f.tick(t, netExecution("a", mc.ActionSucceeded, firstBaseForwardNetOfFeeMicros))
	status, amount, signature := f.forwardState(t, "a")
	if status != "arrived" || amount != "835383" || signature != "net-credit" {
		t.Fatalf("forward = %s %s %q, want arrived 835383 on the net credit", status, amount, signature)
	}
	if got := f.forwarded(t); got != 1 {
		t.Fatalf("deposit.evm_forwarded events = %d, want 1", got)
	}
	f.tick(t, netExecution("a", mc.ActionSucceeded, firstBaseForwardNetOfFeeMicros))
	if status, _, signature := f.forwardState(t, "a"); status != "arrived" || signature != "net-credit" {
		t.Fatalf("after a replay: %s %q, want it to stay arrived on the net credit", status, signature)
	}
}

func TestEVMDepositForward_CreditClaimedByOneForwardIsNotClaimedAgain(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	created := f.clock.Now()
	f.tick(t, netExecution("a", mc.ActionPending, firstBaseForwardInMicros),
		netExecution("b", mc.ActionPending, firstBaseForwardInMicros))
	f.deposit(t, "net-credit", firstBaseForwardNetOfFeeMicros, created.Add(time.Minute))
	f.deposit(t, "a-day-later", firstBaseForwardNetOfFeeMicros, created.Add(app.EVMDepositArrivalWindow+time.Minute))
	f.clock.Advance(2 * time.Minute)
	for range 2 {
		f.tick(t, netExecution("a", mc.ActionSucceeded, firstBaseForwardNetOfFeeMicros),
			netExecution("b", mc.ActionSucceeded, firstBaseForwardNetOfFeeMicros))
	}
	_, _, first := f.forwardState(t, "a")
	status, _, second := f.forwardState(t, "b")
	if first != "net-credit" || status != "pending" || second != "" {
		t.Fatalf("a on %q, b %s on %q: want a on the credit and b still pending", first, status, second)
	}
	if got := f.forwards(t, "arrived"); got != 1 {
		t.Fatalf("arrived forwards = %d, want one per credit", got)
	}
}
