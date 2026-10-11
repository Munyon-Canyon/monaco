package multichaintest_test

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	evmAddr      = "0x5aaeb6053f3e94c9b9a09f33669435e7ef1beaed"
	otherEVMAddr = "0xfb6916095ca1df60bb79ce92ce3ea74c37c5d359"
	mixedCase    = "0x5aAeb6053F3E94C9b9A09f33669435E7Ef1BeAed"
)

type stmt struct {
	name string
	sql  string
	args []any
	ok   bool
}

func run(t *testing.T, pool *pgxpool.Pool, stmts []stmt) {
	t.Helper()
	for _, s := range stmts {
		if _, err := pool.Exec(t.Context(), s.sql, s.args...); (err == nil) != s.ok {
			t.Errorf("%s: err = %v, want ok %v", s.name, err, s.ok)
		}
	}
}

func TestUserEVMWalletIsAllOrNone(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	u := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	set := `UPDATE user_wallets SET evm_privy_wallet_id = $2, evm_address = $3, evm_created_at = $4 WHERE user_id = $1`
	run(t, pool, []stmt{
		{"address only", set, []any{u.ID.UUID(), nil, evmAddr, nil}, false},
		{"mixed case address", set, []any{u.ID.UUID(), "evm-1", mixedCase, "2026-10-11T00:00:00Z"}, false},
		{"all three", set, []any{u.ID.UUID(), "evm-1", evmAddr, "2026-10-11T00:00:00Z"}, true},
	})
}

func TestWithdrawalDestinationMatchesItsKind(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	u := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	insert := `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at,
		destination_kind, network, evm_address, privy_action_id, submitted_at)
		VALUES (gen_random_uuid(), $1, 1000000, $2, $3, now(), $4, $5, $6, $7, $8)`
	legacy := `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, created_at)
		VALUES (gen_random_uuid(), $1, 1000000, $2, 'created', now())`
	id := u.ID.UUID()
	run(t, pool, []stmt{
		{"legacy solana row", legacy, []any{id, string(u.Address)}, true},
		{"evm created", insert, []any{id, evmAddr, "created", "evm", "base", evmAddr, nil, nil}, true},
		{"evm without address", insert, []any{id, evmAddr, "created", "evm", "base", nil, nil, nil}, false},
		{"evm on solana", insert, []any{id, evmAddr, "created", "evm", "solana", evmAddr, nil, nil}, false},
		{"solana on base", insert, []any{id, string(u.Address), "created", "solana", "base", nil, nil, nil}, false},
		{
			"evm submitted with action", insert,
			[]any{id, evmAddr, "submitted", "evm", "base", evmAddr, "action-1", "2026-10-11T00:00:00Z"},
			true,
		},
		{
			"evm submitted without action", insert,
			[]any{id, evmAddr, "submitted", "evm", "base", evmAddr, nil, "2026-10-11T00:00:00Z"},
			false,
		},
		{
			"solana submitted without tx", insert,
			[]any{id, string(u.Address), "submitted", "solana", "solana", nil, nil, "2026-10-11T00:00:00Z"},
			false,
		},
	})
}

func TestAssetListingShape(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	insert := `INSERT INTO asset_listings (id, issuer, symbol, network, chain_id, token_address, decimals,
		shares_per_token, venue, enabled, verified_at, created_at)
		VALUES (gen_random_uuid(), 'ondo', 'AAPLon', $1, $2, $3, $4, 1, $5, $6, $7, now())`
	run(t, pool, []stmt{
		{"evm verified", insert, []any{"ethereum", 1, evmAddr, 18, "uniswap", true, "2026-10-11T00:00:00Z"}, true},
		{"solana", insert, []any{
			"solana", nil, "So11111111111111111111111111111111111111112", 9, "jupiter", true,
			"2026-10-11T00:00:00Z",
		}, true},
		{"unverified disabled", insert, []any{"ethereum", 1, otherEVMAddr, nil, "uniswap", false, nil}, true},
		{"unverified enabled", insert, []any{"ethereum", 1, evmAddr, 18, "uniswap", true, nil}, false},
		{"evm without chain id", insert, []any{"ethereum", nil, evmAddr, 18, "uniswap", false, nil}, false},
		{"evm mixed case", insert, []any{"ethereum", 1, mixedCase, 18, "uniswap", false, nil}, false},
		{"unknown network", insert, []any{"polygon", 137, evmAddr, 18, "uniswap", false, nil}, false},
	})
}

func TestTradeStepsAreUniquePerStep(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM pg_constraint
		WHERE conrelid = 'trade_steps'::regclass AND contype = 'u'`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("unique constraints on trade_steps = %d, %v, want 2 (trade_id, step) and privy_action_id", n, err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM information_schema.columns
		WHERE table_name = 'cabal_positions' AND column_name = 'listing_id' AND is_nullable = 'YES'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("cabal_positions.listing_id nullable columns = %d, %v", n, err)
	}
}
