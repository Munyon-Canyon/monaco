package cabal_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	checkViolation      = "23514"
	uniqueViolation     = "23505"
	foreignKeyViolation = "23503"
)

func run(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) error {
	t.Helper()
	_, err := pool.Exec(t.Context(), sql, args...)
	return err
}

func wantViolation(t *testing.T, what string, err error, code, constraint string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != code || pg.ConstraintName != constraint {
		t.Errorf("%s: err = %v, want SQLSTATE %s on %s", what, err, code, constraint)
	}
}

func wantNoError(t *testing.T, what string, err error) {
	t.Helper()
	if err != nil {
		t.Errorf("%s: %v", what, err)
	}
}

func newID() uuid.UUID { return ids.Real{}.NewV7() }

func TestCabalsSchema_refusesAValueOutsideItsSet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	for _, tt := range []struct {
		column     string
		value      any
		constraint string
	}{
		{"join_mode", "invite_only", "cabals_join_mode_check"},
		{"voter_mode", "some", "cabals_voter_mode_check"},
		{"threshold", "plurality", "cabals_threshold_check"},
		{"proposal_expiry_seconds", 7200, "cabals_proposal_expiry_seconds_check"},
		{"slippage_bps", 0, "cabals_slippage_bps_check"},
		{"slippage_bps", 301, "cabals_slippage_bps_check"},
		{"status", "paused", "cabals_status_check"},
		{"name", "ab", "cabals_name_check"},
		{"name", strings.Repeat("x", 41), "cabals_name_check"},
	} {
		err := run(t, pool, `UPDATE cabals SET `+tt.column+` = $1 WHERE id = $2`, tt.value, c.ID.UUID())
		wantViolation(t, fmt.Sprintf("%s = %v", tt.column, tt.value), err, checkViolation, tt.constraint)
	}
}

func TestCabalsSchema_acceptsTheEdgesOfEachSet(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	for _, tt := range []struct {
		column string
		value  any
	}{
		{"name", "abc"},
		{"name", strings.Repeat("x", 40)},
		{"name", "ééé"},
		{"name", strings.Repeat("é", 40)},
		{"slippage_bps", 1},
		{"slippage_bps", 300},
		{"proposal_expiry_seconds", 3600},
		{"proposal_expiry_seconds", 604800},
		{"proposal_expiry_seconds", 86400},
		{"join_mode", "request"},
		{"voter_mode", "list"},
		{"threshold", "unanimous"},
		{"status", "banned"},
	} {
		err := run(t, pool, `UPDATE cabals SET `+tt.column+` = $1 WHERE id = $2`, tt.value, c.ID.UUID())
		wantNoError(t, fmt.Sprintf("%s = %v", tt.column, tt.value), err)
	}
}

func TestCabalsSchema_slippageAndStatusHaveDefaults(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	creator := testkit.SeedUser(t, pool, testkit.UserOpts{})
	id := newID()
	if err := run(t, pool, `INSERT INTO cabals (id, name, creator_id, join_mode, voter_mode, threshold,
		proposal_expiry_seconds, created_at, updated_at)
		VALUES ($1, 'defaults', $2, 'open', 'all', 'majority', 3600, now(), now())`,
		id, creator.ID.UUID()); err != nil {
		t.Fatalf("insert: %v", err)
	}
	var (
		slippage int
		status   string
	)
	if err := pool.QueryRow(t.Context(), `SELECT slippage_bps, status FROM cabals WHERE id = $1`, id).
		Scan(&slippage, &status); err != nil || slippage != 100 || status != "active" {
		t.Fatalf("defaults = %d, %q, %v; want 100 and active", slippage, status, err)
	}
}

func TestCabalsSchema_creatorsMustExist(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first := testkit.NewCabal(t, pool)
	err := run(t, pool, `UPDATE cabals SET creator_id = $1 WHERE id = $2`, newID(), first.ID.UUID())
	wantViolation(t, "unknown creator", err, foreignKeyViolation, "cabals_creator_id_fkey")
}

func TestCabalMembersSchema_refusesDuplicatesASecondCreatorAndAnInvalidRole(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithMembers(2))
	newcomer := testkit.SeedUser(t, pool, testkit.UserOpts{})
	insert := `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at) VALUES ($1, $2, $3, true, now())`
	wantViolation(t, "the same member twice",
		run(t, pool, insert, c.ID.UUID(), c.Members[1].ID.UUID(), "member"), uniqueViolation, "cabal_members_pkey")
	wantViolation(
		t,
		"a second creator",
		run(
			t,
			pool,
			insert,
			c.ID.UUID(),
			newcomer.ID.UUID(),
			"creator",
		),
		uniqueViolation,
		"cabal_members_one_creator_key",
	)
	wantViolation(t, "an admin",
		run(t, pool, insert, c.ID.UUID(), newcomer.ID.UUID(), "admin"), checkViolation, "cabal_members_role_check")
	wantViolation(t, "a creator who cannot vote",
		run(t, pool, `UPDATE cabal_members SET can_vote = false WHERE cabal_id = $1 AND role = 'creator'`, c.ID.UUID()),
		checkViolation, "cabal_members_creator_votes")
	wantNoError(t, "a member who cannot vote",
		run(t, pool, `UPDATE cabal_members SET can_vote = false WHERE cabal_id = $1 AND role = 'member'`, c.ID.UUID()))
}

func TestCabalMembersSchema_membersMustBeKnownUsersOfKnownCabals(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	insert := `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at) VALUES ($1, $2, 'member', true, now())`
	wantViolation(t, "unknown user",
		run(t, pool, insert, c.ID.UUID(), newID()), foreignKeyViolation, "cabal_members_user_id_fkey")
	wantViolation(t, "unknown cabal",
		run(t, pool, insert, newID(), c.Creator.ID.UUID()), foreignKeyViolation, "cabal_members_cabal_id_fkey")
}

const insertAccessRequest = `INSERT INTO cabal_access_requests
	(id, cabal_id, user_id, direction, invited_by, status, expires_at, decided_by, created_at, decided_at)
	VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now(), $9)`

func insertRequest(
	t *testing.T,
	pool *pgxpool.Pool,
	cabalID, userID, actorID uuid.UUID,
	direction, status string,
) error {
	t.Helper()
	var invitedBy, expiresAt, decidedBy, decidedAt any
	if direction == "invite" {
		invitedBy, expiresAt = actorID, clock.Real{}.Now().Add(7*24*time.Hour)
	}
	if status != "pending" {
		decidedBy, decidedAt = actorID, clock.Real{}.Now()
	}
	return run(t, pool, insertAccessRequest, newID(), cabalID, userID, direction, invitedBy, status, expiresAt,
		decidedBy, decidedAt)
}

func TestAccessRequestsSchema_aSecondPendingRowForOneUserAndCabalIsRefused(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithJoinMode("request"))
	applicant := testkit.SeedUser(t, pool, testkit.UserOpts{})
	cabalID, userID, creatorID := c.ID.UUID(), applicant.ID.UUID(), c.Creator.ID.UUID()
	wantNoError(
		t,
		"the first pending request",
		insertRequest(t, pool, cabalID, userID, creatorID, "request", "pending"),
	)
	wantViolation(t, "a second pending request",
		insertRequest(t, pool, cabalID, userID, creatorID, "request", "pending"),
		uniqueViolation, "cabal_access_requests_pending_key")
	wantViolation(t, "a pending invite beside the pending request",
		insertRequest(t, pool, cabalID, userID, creatorID, "invite", "pending"),
		uniqueViolation, "cabal_access_requests_pending_key")
}

func TestAccessRequestsSchema_aDecidedRowFreesTheSlotAndKeepsItsHistory(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	applicant := testkit.SeedUser(t, pool, testkit.UserOpts{})
	cabalID, userID, creatorID := c.ID.UUID(), applicant.ID.UUID(), c.Creator.ID.UUID()
	for _, decided := range [][2]string{
		{"request", "denied"}, {"request", "revoked"}, {"request", "approved"}, {"invite", "expired"},
	} {
		wantNoError(t, decided[1]+" "+decided[0],
			insertRequest(t, pool, cabalID, userID, creatorID, decided[0], decided[1]))
	}
	wantNoError(t, "a pending row beside the decided ones",
		insertRequest(t, pool, cabalID, userID, creatorID, "request", "pending"))
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_access_requests WHERE user_id = $1`,
		userID).Scan(&rows); err != nil || rows != 5 {
		t.Fatalf("rows = %d, %v; want the four decided rows and the pending one", rows, err)
	}
}

func TestAccessRequestsSchema_onlyAnInviteCanExpire(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	cabalID, creatorID := c.ID.UUID(), c.Creator.ID.UUID()
	for _, tt := range []struct {
		direction string
		valid     bool
	}{{"invite", true}, {"request", false}} {
		user := testkit.SeedUser(t, pool, testkit.UserOpts{}).ID.UUID()
		err := insertRequest(t, pool, cabalID, user, creatorID, tt.direction, "expired")
		if tt.valid {
			wantNoError(t, "an expired "+tt.direction, err)
			continue
		}
		wantViolation(
			t,
			"an expired "+tt.direction,
			err,
			checkViolation,
			"cabal_access_requests_requests_do_not_expire",
		)
		pending := insertRequest(t, pool, cabalID, user, creatorID, tt.direction, "pending")
		wantNoError(t, "a pending "+tt.direction, pending)
		wantViolation(t, "a "+tt.direction+" moved to expired",
			run(t, pool, `UPDATE cabal_access_requests SET status = 'expired' WHERE cabal_id = $1 AND user_id = $2`,
				cabalID, user),
			checkViolation, "cabal_access_requests_requests_do_not_expire")
	}
}

func TestAccessRequestsSchema_directionDecidesWhoInvitesAndWhetherARowExpires(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	inviter, expiry := c.Creator.ID.UUID(), clock.Real{}.Now().Add(time.Hour)
	for _, tt := range []struct {
		name      string
		direction string
		invitedBy any
		expiresAt any
		valid     bool
	}{
		{"request with neither", "request", nil, nil, true},
		{"invite with both", "invite", inviter, expiry, true},
		{"request with an inviter", "request", inviter, nil, false},
		{"request with an expiry", "request", nil, expiry, false},
		{"invite without an inviter", "invite", nil, expiry, false},
		{"invite without an expiry", "invite", inviter, nil, false},
	} {
		user := testkit.SeedUser(t, pool, testkit.UserOpts{})
		err := run(t, pool, insertAccessRequest, newID(), c.ID.UUID(), user.ID.UUID(), tt.direction, tt.invitedBy,
			"pending", tt.expiresAt, nil, nil)
		if tt.valid {
			wantNoError(t, tt.name, err)
			continue
		}
		wantViolation(t, tt.name, err, checkViolation, "cabal_access_requests_direction_shape")
	}
}

func TestAccessRequestsSchema_refusesAnUnknownDirectionStatusOrReference(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{})
	insert := func(cabalID, userID uuid.UUID, direction, status string, invitedBy, decidedBy any) error {
		return run(t, pool, insertAccessRequest, newID(), cabalID, userID, direction, invitedBy, status, nil,
			decidedBy, nil)
	}
	wantViolation(t, "direction", insert(c.ID.UUID(), user.ID.UUID(), "ask", "pending", nil, nil),
		checkViolation, "cabal_access_requests_direction_check")
	wantViolation(t, "status", insert(c.ID.UUID(), user.ID.UUID(), "request", "waiting", nil, nil),
		checkViolation, "cabal_access_requests_status_check")
	wantViolation(t, "unknown cabal", insert(newID(), user.ID.UUID(), "request", "pending", nil, nil),
		foreignKeyViolation, "cabal_access_requests_cabal_id_fkey")
	wantViolation(t, "unknown user", insert(c.ID.UUID(), newID(), "request", "pending", nil, nil),
		foreignKeyViolation, "cabal_access_requests_user_id_fkey")
	wantViolation(t, "unknown decider", insert(c.ID.UUID(), user.ID.UUID(), "request", "denied", nil, newID()),
		foreignKeyViolation, "cabal_access_requests_decided_by_fkey")
	err := run(t, pool, insertAccessRequest, newID(), c.ID.UUID(), user.ID.UUID(), "invite", newID(), "pending",
		clock.Real{}.Now(), nil, nil)
	wantViolation(t, "unknown inviter", err, foreignKeyViolation, "cabal_access_requests_invited_by_fkey")
}

func TestTreasuryWalletsSchema_oneWalletPerCabalAndNoWalletIsShared(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	first := testkit.NewCabal(t, pool)
	second := testkit.NewCabal(t, pool)
	insert := `INSERT INTO treasury_wallets (cabal_id, privy_wallet_id, address, created_at) VALUES ($1, $2, $3, now())`
	wantViolation(t, "a second wallet for one cabal",
		run(t, pool, insert, first.ID.UUID(), "wallet-x", "address-x"), uniqueViolation, "treasury_wallets_pkey")
	wantViolation(t, "a shared Privy wallet",
		run(t, pool, `UPDATE treasury_wallets SET privy_wallet_id = $1 WHERE cabal_id = $2`,
			first.PrivyWalletID, second.ID.UUID()), uniqueViolation, "treasury_wallets_privy_wallet_id_key")
	wantViolation(t, "a shared address",
		run(t, pool, `UPDATE treasury_wallets SET address = $1 WHERE cabal_id = $2`,
			string(first.TreasuryAddress), second.ID.UUID()), uniqueViolation, "treasury_wallets_address_key")
	wantViolation(t, "a wallet for an unknown cabal",
		run(t, pool, insert, newID(), "wallet-y", "address-y"), foreignKeyViolation, "treasury_wallets_cabal_id_fkey")
}
