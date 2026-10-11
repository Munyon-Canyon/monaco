package funding_test

import (
	"database/sql"
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	mc "github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func forwardOf(id string, state mc.ActionState, input, output uint64) mc.Execution {
	ex := execution(id, state)
	ex.InputMicros, ex.AmountMicros = money.MicrosFromUint64(input), money.MicrosFromUint64(output)
	return ex
}

func (f *forwardFixture) fee(t *testing.T, id string) sql.NullString {
	t.Helper()
	var fee sql.NullString
	if err := f.pool.QueryRow(t.Context(), `SELECT fee_micros::text FROM evm_deposit_forwards
		WHERE privy_execution_id = $1`, id).Scan(&fee); err != nil {
		t.Fatal(err)
	}
	return fee
}

func TestEVMDepositForward_RecordsTheFeeOnceTheForwardSucceeds(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	f.tick(t, forwardOf("pending", mc.ActionPending, 999_250, 999_250))
	if got := f.fee(t, "pending"); got.Valid {
		t.Fatalf("pending forward fee = %v, want none before the output is known", got.String)
	}
	f.tick(t,
		forwardOf("pending", mc.ActionSucceeded, 999_250, 835_383),
		forwardOf("over", mc.ActionSucceeded, 1_000_000, 1_000_001),
		forwardOf("no-input", mc.ActionSucceeded, 0, 835_383),
	)
	for id, want := range map[string]sql.NullString{
		"pending":  {String: "163867", Valid: true},
		"over":     {String: "0", Valid: true},
		"no-input": {},
	} {
		if got := f.fee(t, id); got != want {
			t.Errorf("%s fee = %+v, want %+v", id, got, want)
		}
	}
}

func TestLatestEVMDepositForwardFees_takesTheNewestArrivedForwardPerNetwork(t *testing.T) {
	t.Parallel()
	f := newForwardFixture(t)
	arb := forwardOf("arb", mc.ActionSucceeded, 1_000_000, 990_000)
	arb.Network = chain.NetworkArbitrum
	f.tick(t,
		forwardOf("old", mc.ActionSucceeded, 1_000_000, 800_000),
		forwardOf("new", mc.ActionSucceeded, 2_000_000, 1_836_902),
		forwardOf("unarrived", mc.ActionSucceeded, 1_000_000, 1),
		forwardOf("unknown", mc.ActionSucceeded, 0, 500_000),
		arb,
	)
	now := f.clock.Now()
	for id, at := range map[string]time.Time{
		"old": now.Add(-time.Hour), "new": now.Add(-time.Minute), "unknown": now, "arb": now,
	} {
		if _, err := f.pool.Exec(t.Context(), `UPDATE evm_deposit_forwards SET status = 'arrived', arrived_at = $2
			WHERE privy_execution_id = $1`, id, at); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := sqlc.New(f.pool).LatestEVMDepositForwardFees(t.Context())
	want := []sqlc.LatestEVMDepositForwardFeesRow{
		{Network: string(chain.NetworkArbitrum), FeeMicros: "10000"},
		{Network: string(chain.NetworkBase), FeeMicros: "163098"},
	}
	if err != nil || !slices.Equal(rows, want) {
		t.Fatalf("LatestEVMDepositForwardFees = %+v, %v, want %+v", rows, err, want)
	}
}
