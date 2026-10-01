package cabal_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func seedManyCabals(t *testing.T, pool *pgxpool.Pool, creator ids.UserID, n int) []ids.CabalID {
	t.Helper()
	raw, codes, wallets, addresses := make([]uuid.UUID, n), make([]string, n), make([]string, n), make([]string, n)
	asked := make([]ids.CabalID, n)
	for i := range n {
		raw[i] = ids.Real{}.NewV7()
		asked[i] = ids.CabalIDFrom(raw[i])
		codes[i] = fmt.Sprintf("B%09d", i)
		wallets[i], addresses[i] = fmt.Sprintf("bulk-wallet-%d", i), fmt.Sprintf("bulk-address-%d", i)
	}
	now := clock.Real{}.Now().UTC()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold,
		proposal_expiry_seconds, invite_code, created_at, updated_at)
		SELECT id, 'Bulk pot', $2, 'open', 'all', 'majority', 86400, code, $3, $3
		FROM unnest($1::uuid[], $4::text[]) AS c (id, code)`, raw, creator.UUID(), now, codes); err != nil {
		t.Fatalf("insert cabals: %v", err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		SELECT id, wallet, address, $4 FROM unnest($1::uuid[], $2::text[], $3::text[]) AS w (id, wallet, address)`,
		raw, wallets, addresses, now); err != nil {
		t.Fatalf("insert treasury wallets: %v", err)
	}
	return asked
}

func TestQueries_everyMethodIsOneRoundTripAndTreasuryWalletsServes1000CabalsInOne(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	w := newContractWorld(t)
	seedContractWorld(t, pool, w)
	const bulk = 1000
	asked := seedManyCabals(t, pool, w.users[0], bulk)
	q := cabal.New(module.Deps{Pool: pool}).Queries()
	ctx := t.Context()
	a, user := w.id("a"), w.users[0]
	for name, call := range map[string]func() error{
		"Cabal": func() error { _, err := q.Cabal(ctx, a); return err },
		"Cabals 1000 ids": func() error {
			got, err := q.Cabals(ctx, asked)
			if err == nil && len(got) != bulk {
				return fmt.Errorf("Cabals returned %d cabals, want %d", len(got), bulk)
			}
			return err
		},
		"IsMember":    func() error { _, err := q.IsMember(ctx, a, user); return err },
		"Member":      func() error { _, err := q.Member(ctx, a, user); return err },
		"Members":     func() error { _, err := q.Members(ctx, a); return err },
		"VoterSet":    func() error { _, err := q.VoterSet(ctx, a); return err },
		"Rules":       func() error { _, err := q.Rules(ctx, a); return err },
		"SlippageBps": func() error { _, err := q.SlippageBps(ctx, a); return err },
		"Status":      func() error { _, err := q.Status(ctx, a); return err },
		"TreasuryWallet": func() error {
			_, err := q.TreasuryWallet(ctx, a)
			return err
		},
		"TreasuryWallets over 1000 cabals": func() error {
			got, err := q.TreasuryWallets(ctx)
			if err == nil && len(got) != bulk+len(w.cabals) {
				return fmt.Errorf("TreasuryWallets returned %d wallets, want %d", len(got), bulk+len(w.cabals))
			}
			return err
		},
		"CabalsOf": func() error { _, err := q.CabalsOf(ctx, user); return err },
	} {
		testkit.AssertQueries(t, name, func() {
			if err := call(); err != nil {
				t.Errorf("%s: %v", name, err)
			}
		})
	}
}

func TestQueries_aRowWhoseIDIsNotV7IsDecodeFailed(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx := t.Context()
	w := newContractWorld(t)
	seedContractWorld(t, pool, w)
	now := clock.Real{}.Now().UTC()
	var v4User, v4Cabal uuid.UUID
	if err := pool.QueryRow(ctx, `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at,
		created_at, updated_at) VALUES (gen_random_uuid(), 'did:privy:v4', 'sms', $1, $1, $1) RETURNING id`,
		now).Scan(&v4User); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold,
		proposal_expiry_seconds, invite_code, created_at, updated_at)
		VALUES (gen_random_uuid(), 'V4 pot', $1, 'open', 'all', 'majority', 86400, 'V400000000', $2, $2)
		RETURNING id`, w.users[0].UUID(), now).Scan(&v4Cabal); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at)
		VALUES ($1, 'v4-wallet', 'v4-address', $2)`, v4Cabal, now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		VALUES ($1, $2, 'member', true, $3), ($4, $5, 'creator', true, $3)`,
		w.id("c").UUID(), v4User, now, v4Cabal, w.users[0].UUID()); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE cabals SET creator_id = $1 WHERE id = $2`,
		v4User, w.id("d").UUID()); err != nil {
		t.Fatal(err)
	}
	q := cabal.New(module.Deps{Pool: pool}).Queries()
	_, byV4Cabal := q.Cabal(ctx, ids.CabalIDFrom(v4Cabal))
	_, byV4Creator := q.Cabal(ctx, w.id("d"))
	_, batch := q.Cabals(ctx, []ids.CabalID{w.id("a"), ids.CabalIDFrom(v4Cabal)})
	_, members := q.Members(ctx, w.id("c"))
	_, voters := q.VoterSet(ctx, w.id("c"))
	_, wallets := q.TreasuryWallets(ctx)
	_, cabals := q.CabalsOf(ctx, w.users[0])
	for name, err := range map[string]error{
		"Cabal with a v4 cabal id": byV4Cabal, "Cabal with a v4 creator": byV4Creator, "Cabals": batch,
		"Members": members, "VoterSet": voters, "TreasuryWallets": wallets, "CabalsOf": cabals,
	} {
		if errs.CodeOf(err) != errs.CodeDecodeFailed {
			t.Errorf("%s err = %v, want decode_failed", name, err)
		}
	}
}

func TestQueries_databaseFailuresAreInternalAndKeepTheirCause(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := cabal.New(module.Deps{Pool: pool}).Queries()
	id, user := ids.CabalID{}, ids.UserID{}
	for name, call := range map[string]func() error{
		"Cabal":          func() error { _, err := q.Cabal(ctx, id); return err },
		"Cabals":         func() error { _, err := q.Cabals(ctx, []ids.CabalID{id}); return err },
		"IsMember":       func() error { _, err := q.IsMember(ctx, id, user); return err },
		"Member":         func() error { _, err := q.Member(ctx, id, user); return err },
		"Members":        func() error { _, err := q.Members(ctx, id); return err },
		"VoterSet":       func() error { _, err := q.VoterSet(ctx, id); return err },
		"Rules":          func() error { _, err := q.Rules(ctx, id); return err },
		"SlippageBps":    func() error { _, err := q.SlippageBps(ctx, id); return err },
		"Status":         func() error { _, err := q.Status(ctx, id); return err },
		"TreasuryWallet": func() error { _, err := q.TreasuryWallet(ctx, id); return err },
		"TreasuryWallets": func() error {
			_, err := q.TreasuryWallets(ctx)
			return err
		},
		"CabalsOf": func() error { _, err := q.CabalsOf(ctx, user); return err },
	} {
		if err := call(); errs.CodeOf(err) != errs.CodeInternal || !errors.Is(err, context.Canceled) {
			t.Errorf("%s err = %v, want internal wrapping the context error", name, err)
		}
	}
}
