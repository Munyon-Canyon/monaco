package funding_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
)

func TestDepositCursorMigration_ReplaysFromTheOldCursorAndCreditsEachDepositOnce(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	ctx := t.Context()
	const cursorSlot = 100
	execAll(t, f.pool,
		`CREATE TABLE IF NOT EXISTS deposit_cursors (
			wallet_address text PRIMARY KEY, last_signature text, cursor_slot bigint NOT NULL DEFAULT 0,
			scanned_at timestamptz NOT NULL)`,
		fmt.Sprintf(`INSERT INTO deposit_cursors (wallet_address, last_signature, cursor_slot, scanned_at)
			VALUES ('%s', 'cursor', %d, now())`, f.user.Address, cursorSlot),
		fmt.Sprintf(`INSERT INTO deposits (id, user_id, wallet_address, tx_signature, amount_micros, slot, credited_at)
			VALUES (gen_random_uuid(), '%s', '%s', 'sigCredited', 5000000, 120, now())`, f.user.ID.UUID(), f.user.Address),
	)
	migration, err := os.ReadFile("../../../migrations/20261009000000_funding_copy_deposit_cursors.sql")
	if err != nil {
		t.Fatal(err)
	}
	execAll(t, f.pool, string(migration), string(migration))
	if n := f.count(t, `SELECT count(*) FROM deposit_watch_wallets WHERE first_seen_slot = $1`, cursorSlot); n != 1 {
		t.Fatalf("watch wallets at the cursor slot = %d, want 1 after running the migration twice", n)
	}
	history := []solana.SignatureInfo{
		{Signature: "sigAbove", Slot: 150, BlockTime: f.now},
		{Signature: "sigCredited", Slot: 120, BlockTime: f.now},
		{Signature: "sigBelow", Slot: 50, BlockTime: f.now},
	}
	ata := canonicalAccount(t, f.user.Address)
	rpc := &watchRPC{
		pool: f.pool, slot: 200, accounts: []solana.TokenAccountState{openAccount(ata, f.user.Address, 5_000_000)},
		signaturesFor: func(before, until chain.Signature, limit int) []solana.SignatureInfo {
			return signaturesFromHistory(history, before, until, limit)
		},
	}
	watch := watchFor(f.pool, f.user, f.now.Add(time.Minute), rpc, unlimited(), 420, 421)
	for range 2 {
		if _, err := watch.Tick(watchActor(t)); err != nil {
			t.Fatal(err)
		}
	}
	assertCandidateSignatures(ctx, t, f.pool, f.user.Address, "sigAbove", "sigCredited")
	resolveAllSeen(t, f)
	for sig, want := range map[string]int{"sigAbove": 1, "sigCredited": 1, "sigBelow": 0} {
		if got := f.count(t, `SELECT count(*) FROM deposits WHERE tx_signature = $1`, sig); got != want {
			t.Fatalf("deposits for %s = %d, want %d", sig, got, want)
		}
	}
	if got := f.count(t, `SELECT count(*) FROM events WHERE type = $1`, events.TypeDepositCredited); got != 1 {
		t.Fatalf("deposit.credited events = %d, want 1", got)
	}
}

func resolveAllSeen(t *testing.T, f candidateFixture) {
	t.Helper()
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(5_000_000, 6),
	}}})
	rows, err := f.pool.Query(
		t.Context(), `SELECT id, payload FROM events WHERE type = $1 ORDER BY id`, events.TypeDepositCandidateSeen,
	)
	if err != nil {
		t.Fatal(err)
	}
	type seen struct {
		id      uuid.UUID
		payload []byte
	}
	var all []seen
	for rows.Next() {
		var s seen
		if err := rows.Scan(&s.id, &s.payload); err != nil {
			t.Fatal(err)
		}
		all = append(all, s)
	}
	rows.Close()
	if len(all) != 2 {
		t.Fatalf("candidate_seen events = %d, want 2", len(all))
	}
	for _, s := range all {
		m := &candidateDelivery{
			Msg:  chaos.NewMsg(d.conn, events.TypeDepositCandidateSeen, ids.EventIDFrom(s.id), s.payload),
			seen: 1,
		}
		d.reg.Dispatch(observability.WithActor(t.Context(), "system:worker"), "funding", m)
		if m.outcome != bus.OutcomeAck {
			t.Fatalf("delivery = %q, want ack", m.outcome)
		}
	}
}

func execAll(t *testing.T, pool *pgxpool.Pool, statements ...string) {
	t.Helper()
	for _, statement := range statements {
		if _, err := pool.Exec(context.Background(), statement); err != nil {
			t.Fatal(err)
		}
	}
}
