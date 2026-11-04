package social_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type backfillSeed struct {
	cabal, swap, proposal, asset uuid.UUID
	members                      [2]uuid.UUID
	confirmedAt                  time.Time
}

func seedBackfill(t *testing.T, pool *pgxpool.Pool) backfillSeed {
	t.Helper()
	cabal := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	s := backfillSeed{
		cabal: cabal.ID.UUID(), swap: newID(), proposal: newID(), asset: newID(),
		members:     [2]uuid.UUID{cabal.Members[0].ID.UUID(), cabal.Members[1].ID.UUID()},
		confirmedAt: clock.Real{}.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond),
	}
	exec(t, pool, `INSERT INTO assets
		(id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, 'AAPLx', 'mint-aapl', 6, 'xstocks', 'equity', 'Apple', true, 'aapl', now(), now())`, s.asset)
	exec(t, pool, `INSERT INTO swaps
		(id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint, out_mint, in_amount,
		 out_amount, slippage_bps, status, created_at, confirmed_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'buy', 'AAPLx', 'mint-usdc', 'mint-aapl', 393500000,
		 3000000, 100, 'confirmed', $4, $4, $4)`, s.swap, s.proposal, s.cabal, s.confirmedAt)
	exec(t, pool, `INSERT INTO swaps
		(id, source_kind, source_id, cabal_id, treasury_address, action, symbol, in_mint, out_mint, in_amount,
		 slippage_bps, status, created_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'buy', 'AAPLx', 'mint-usdc', 'mint-aapl', 100, 100,
		 'created', $4, $4)`, newID(), newID(), s.cabal, s.confirmedAt)
	return s
}

func newID() uuid.UUID { return ids.Real{}.NewV7() }

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatalf("exec %.60s: %v", sql, err)
	}
}

func runBackfill(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	paths, err := filepath.Glob("../../../migrations/*_social_chat_moments_backfill.sql")
	if err != nil || len(paths) != 1 {
		t.Fatalf("migration files = %v, err = %v, want exactly one", paths, err)
	}
	raw, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	exec(t, pool, string(raw))
}

func countSystem(t *testing.T, pool *pgxpool.Pool, cabal uuid.UUID, kind string) int {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM cabal_messages WHERE cabal_id = $1 AND kind = 'system' AND moment ->> 'kind' = $2`,
		cabal, kind).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestChatMomentsBackfill_WritesOneRowPerFactWithSourceTimestamps(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := seedBackfill(t, pool)
	runBackfill(t, pool)

	for kind, want := range map[string]int{"cabal_created": 1, "member_joined": 2, "trade_filled": 1} {
		if got := countSystem(t, pool, s.cabal, kind); got != want {
			t.Fatalf("%s rows = %d, want %d", kind, got, want)
		}
	}
	var shaped int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_messages
		WHERE cabal_id = $1 AND author_id IS NULL AND body IS NULL AND parent_id IS NULL`,
		s.cabal).Scan(&shaped); err != nil || shaped != 4 {
		t.Fatalf("shaped rows = %d, err = %v, want 4 system rows", shaped, err)
	}
}

func TestChatMomentsBackfill_UsesSourceTimestamps(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := seedBackfill(t, pool)
	runBackfill(t, pool)

	var createdAt, cabalAt time.Time
	if err := pool.QueryRow(t.Context(), `SELECT m.created_at, c.created_at FROM cabal_messages m
		JOIN cabals c ON c.id = m.cabal_id WHERE m.cabal_id = $1 AND m.dedupe_key = 'created'`, s.cabal).
		Scan(&createdAt, &cabalAt); err != nil || !createdAt.Equal(cabalAt) {
		t.Fatalf("created row at %v, cabal at %v, err = %v", createdAt, cabalAt, err)
	}
	var joinedMatches int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_messages m
		JOIN cabal_members cm ON cm.cabal_id = m.cabal_id AND m.dedupe_key = 'joined:' || cm.user_id::text
		WHERE m.cabal_id = $1 AND m.created_at = cm.joined_at`, s.cabal).Scan(&joinedMatches); err != nil || joinedMatches != 2 {
		t.Fatalf("joined rows at member joined_at = %d, err = %v, want 2", joinedMatches, err)
	}
}

func TestChatMomentsBackfill_FillParsesWithTheLiveShape(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := seedBackfill(t, pool)
	runBackfill(t, pool)
	var raw []byte
	var at time.Time
	if err := pool.QueryRow(t.Context(), `SELECT moment, created_at FROM cabal_messages
		WHERE cabal_id = $1 AND dedupe_key = 'fill:' || $2::text`, s.cabal, s.swap).Scan(&raw, &at); err != nil {
		t.Fatal(err)
	}
	moment, err := domain.ParseChatMoment(raw)
	if err != nil {
		t.Fatalf("the backfilled fill does not parse: %v (%s)", err, raw)
	}
	want := domain.TradeFilled{
		Side: domain.TradeBuy, Symbol: "AAPLx", AssetID: s.asset, PriceMicros: 131_166_666,
		USDCMicros: 393_500_000, FilledAt: s.confirmedAt, SwapID: s.swap, ProposalID: s.proposal,
	}
	if got := moment.Trade(); got != want || !at.Equal(s.confirmedAt) {
		t.Fatalf("fill = %+v at %v, want %+v at %v", got, at, want, s.confirmedAt)
	}
}

func TestChatMomentsBackfill_SecondRunAddsNothing(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := seedBackfill(t, pool)
	runBackfill(t, pool)
	before := countRows(t, pool, s.cabal)
	runBackfill(t, pool)
	if after := countRows(t, pool, s.cabal); after != before || before != 4 {
		t.Fatalf("rows went %d -> %d, want 4 and unchanged", before, after)
	}
}

func TestChatMomentsBackfill_SkipsMembersWhoAlreadyHaveALiveJoinRow(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	s := seedBackfill(t, pool)
	exec(t, pool, `INSERT INTO cabal_messages (id, cabal_id, kind, moment, dedupe_key, created_at)
		VALUES ($1, $2, 'system', jsonb_build_object('kind', 'member_joined', 'user_id', $3::uuid), $4, now())`,
		newID(), s.cabal, s.members[1], "joined:"+s.members[1].String()+":"+newID().String())
	runBackfill(t, pool)
	var n int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_messages
		WHERE cabal_id = $1 AND moment ->> 'kind' = 'member_joined' AND moment ->> 'user_id' = $2`,
		s.cabal, s.members[1].String()).Scan(&n); err != nil || n != 1 {
		t.Fatalf("member with a live join row has %d join rows, err = %v, want 1", n, err)
	}
	if got := countSystem(t, pool, s.cabal, "member_joined"); got != 2 {
		t.Fatalf("member_joined rows = %d, want 2", got)
	}
}

func countRows(t *testing.T, pool *pgxpool.Pool, cabal uuid.UUID) int {
	t.Helper()
	var n int
	err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_messages WHERE cabal_id = $1`, cabal).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
