package treasury_test

import (
	"context"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestUserLedgerProperty_replaysKeepDepositsBalanced(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	rapid.Check(t, func(rt *rapid.T) {
		deposits, want := replayDeposits(rt, f, user, h)
		assertDepositTxnsBalance(rt, f, deposits)
		assertWalletDepositSum(rt, f, deposits, want)
	})
}

func replayDeposits(rt *rapid.T, f fixture, user ids.UserID, h adapters.UserLedger) ([][16]byte, uint64) {
	amounts := rapid.SliceOfN(rapid.Uint64Range(1, 1_000_000_000), 1, 8).Draw(rt, "amounts")
	deposits := make([][16]byte, 0, len(amounts))
	var total uint64
	for _, amount := range amounts {
		depositID := f.ids.NewV7()
		deposits = append(deposits, depositID)
		total += amount
		e := events.DepositCredited{
			V: 1, DepositID: depositID, UserID: user.UUID(), TxSignature: chain.Signature(depositID.String()),
			AmountMicros: money.MicrosFromUint64(amount),
		}
		for range rapid.IntRange(1, 3).Draw(rt, "replays") {
			if err := f.do(func(ctx context.Context, tx db.Tx) error {
				return h.Handle(ctx, tx, e, f.clock.Now())
			}); err != nil {
				rt.Fatal(err)
			}
		}
	}
	return deposits, total
}

func assertDepositTxnsBalance(rt *rapid.T, f fixture, deposits [][16]byte) {
	var unbalanced int
	if err := f.pool.QueryRow(f.t.Context(), `SELECT count(*) FROM (
		SELECT txn_id, asset FROM user_txn_entries WHERE txn_id = ANY($1)
		GROUP BY txn_id, asset HAVING sum(amount) <> 0
	) AS unbalanced`, deposits).Scan(&unbalanced); err != nil {
		rt.Fatal(err)
	}
	if unbalanced != 0 {
		rt.Fatalf("unbalanced txns = %d, want 0", unbalanced)
	}
}

func assertWalletDepositSum(rt *rapid.T, f fixture, deposits [][16]byte, want uint64) {
	var wallet string
	if err := f.pool.QueryRow(f.t.Context(), `SELECT coalesce(sum(amount), 0)::text FROM user_txn_entries
		WHERE txn_id = ANY($1) AND account = 'wallet' AND asset = $2`, deposits, usdc).Scan(&wallet); err != nil {
		rt.Fatal(err)
	}
	if wallet != money.MicrosFromUint64(want).String() {
		rt.Fatalf("wallet = %s, want %d", wallet, want)
	}
}
