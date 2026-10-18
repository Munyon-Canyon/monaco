package multichaintest_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
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

func TestSwapFailureCodesIncludeSwapReverted(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	insert := `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint,
		out_mint, in_amount, slippage_bps, status, failure_code, created_at, updated_at)
		VALUES (gen_random_uuid(), 'proposal', gen_random_uuid(), gen_random_uuid(), $1, 'buy', 'AAPLx', 'usdc',
		'aaplx', 1000000, 100, 'failed', $2, now(), now())`
	stmts := make([]stmt, 0, 1+len(domain.FailureCodes()))
	stmts = append(stmts, stmt{"unknown code", insert, []any{evmAddr, "exploded"}, false})
	for _, code := range domain.FailureCodes() {
		stmts = append(stmts, stmt{string(code), insert, []any{evmAddr, string(code)}, true})
	}
	if !slices.Contains(domain.FailureCodes(), domain.FailureSwapReverted) {
		t.Fatal("swap_reverted is not a failure code")
	}
	run(t, pool, stmts)
}

func TestSwapsAreParkedWithAtAndCodeTogether(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	insert := `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint,
		out_mint, in_amount, slippage_bps, status, parked_at, parked_code, created_at, updated_at)
		VALUES (gen_random_uuid(), 'proposal', gen_random_uuid(), gen_random_uuid(), $1, 'buy', 'AAPLx', 'usdc',
		'aaplx', 1000000, 100, 'created', $2, $3, now(), now())`
	run(t, pool, []stmt{
		{"unparked", insert, []any{evmAddr, nil, nil}, true},
		{"parked", insert, []any{evmAddr, "2026-10-11T00:00:00Z", "asset_untradable"}, true},
		{"parked without a code", insert, []any{evmAddr, "2026-10-11T00:00:00Z", nil}, false},
		{"a code without parking", insert, []any{evmAddr, nil, "asset_untradable"}, false},
	})
}

func TestSwapOperatorActionsCheckTheActionAndTheReasonLength(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var swap string
	if err := pool.QueryRow(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, in_amount, slippage_bps, status, created_at, updated_at)
		VALUES (gen_random_uuid(), 'proposal', gen_random_uuid(), gen_random_uuid(), $1, 'buy', 'AAPLx', 'usdc',
		'aaplx', 1000000, 100, 'created', now(), now()) RETURNING id::text`, evmAddr).Scan(&swap); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO swap_operator_actions (id, swap_id, action, actor, reason, detail, created_at)
		VALUES (gen_random_uuid(), $1, $2, 'operator:alice', $3, '{}', now())`
	long := strings.Repeat("é", 500)
	run(t, pool, []stmt{
		{"unpark", insert, []any{swap, "unpark", "rpc blip"}, true},
		{"fail parked", insert, []any{swap, "fail_parked", "listing gone"}, true},
		{"retry bridge back", insert, []any{swap, "retry_bridge_back", "privy rejected"}, true},
		{"return evm cash", insert, []any{swap, "return_evm_cash", "stranded since 2026-10-12"}, true},
		{"unknown action", insert, []any{swap, "force_resolve", "rpc blip"}, false},
		{"empty reason", insert, []any{swap, "unpark", ""}, false},
		{"500 characters", insert, []any{swap, "unpark", long}, true},
		{"501 characters", insert, []any{swap, "unpark", long + "x"}, false},
	})
}

func TestEVMDepositForwardFeeIsNeverNegative(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	u := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	insert := `INSERT INTO evm_deposit_forwards (id, user_id, network, source_tx, privy_execution_id, amount_micros,
		status, created_at, fee_micros) VALUES (gen_random_uuid(), $1, 'base', '0xsrc', $2, 835383, 'pending', now(), $3)`
	run(t, pool, []stmt{
		{"unknown fee", insert, []any{u.ID.UUID(), "e1", nil}, true},
		{"zero fee", insert, []any{u.ID.UUID(), "e2", 0}, true},
		{"measured fee", insert, []any{u.ID.UUID(), "e3", 163098}, true},
		{"negative fee", insert, []any{u.ID.UUID(), "e4", -1}, false},
	})
}

func TestTradeStepAttemptsStartAtOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	var swap string
	if err := pool.QueryRow(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, in_amount, slippage_bps, status, created_at, updated_at)
		VALUES (gen_random_uuid(), 'proposal', gen_random_uuid(), gen_random_uuid(), $1, 'sell', 'AAPLx', 'aaplx',
		'usdc', 1000000, 100, 'confirmed', now(), now()) RETURNING id::text`, evmAddr).Scan(&swap); err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status, attempt, created_at,
		updated_at) VALUES (gen_random_uuid(), $1, $2, 'base', 1000000, 'pending', $3, now(), now())`
	run(t, pool, []stmt{
		{"attempt 0", insert, []any{swap, "bridge_back", 0}, false},
		{"attempt 2", insert, []any{swap, "bridge_back", 2}, true},
	})
	var attempt int
	if err := pool.QueryRow(t.Context(), `INSERT INTO trade_steps (id, trade_id, step, network, amount_micros, status,
		created_at, updated_at) VALUES (gen_random_uuid(), $1, 'swap', 'base', 1000000, 'pending', now(), now())
		RETURNING attempt`, swap).Scan(&attempt); err != nil || attempt != 1 {
		t.Fatalf("default attempt = %d, %v, want 1", attempt, err)
	}
}
