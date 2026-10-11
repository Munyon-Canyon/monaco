package funding_test

import (
	"context"
	"maps"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestEstimatedDepositFees_ReadsTheLastArrivedForwardFeePerNetwork(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	m := funding.New(module.Deps{Pool: pool})
	if fees, err := m.EstimatedDepositFees(t.Context()); err != nil || len(fees) != 0 {
		t.Fatalf("with no forwards fees = %v, %v, want none", fees, err)
	}
	insert := `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx, privy_execution_id, amount_micros,
		status, solana_signature, created_at, arrived_at, fee_micros)
		VALUES (gen_random_uuid(), $1, $2, '0xsrc', $3, 835383, 'arrived', $3, now(), now() - $4::interval, $5)`
	for _, row := range [][]any{
		{"base", "old", "1 hour", 900_000},
		{"base", "new", "1 minute", 163_098},
		{"arbitrum", "arb", "1 minute", 0},
	} {
		if _, err := pool.Exec(t.Context(), insert, append([]any{user.ID.UUID()}, row...)...); err != nil {
			t.Fatal(err)
		}
	}
	fees, err := m.EstimatedDepositFees(t.Context())
	want := map[chain.Network]money.Micros{
		chain.NetworkBase: money.MicrosFromUint64(163_098), chain.NetworkArbitrum: money.MicrosFromUint64(0),
	}
	if err != nil || !maps.Equal(fees, want) {
		t.Fatalf("EstimatedDepositFees = %v, %v, want %v", fees, err, want)
	}
}

func TestEstimatedDepositFees_FailsOnAReadErrorAndOnAFeeBeyondMicros(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	m := funding.New(module.Deps{Pool: pool})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := m.EstimatedDepositFees(canceled); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("canceled read err = %v, want %s", err, errs.CodeInternal)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx,
		privy_execution_id, amount_micros, status, solana_signature, created_at, arrived_at, fee_micros)
		VALUES (gen_random_uuid(), $1, 'base', '0xsrc', 'e1', 1, 'arrived', 'sig', now(), now(), 99999999999999999999)`,
		user.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := m.EstimatedDepositFees(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("oversized fee err = %v, want %s", err, errs.CodeDecodeFailed)
	}
}
