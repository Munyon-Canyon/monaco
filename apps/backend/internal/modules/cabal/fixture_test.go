package cabal_test

import (
	"regexp"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type memberRow struct {
	userID  string
	role    string
	canVote bool
}

func memberRows(t *testing.T, pool *pgxpool.Pool, c testkit.SeededCabal) []memberRow {
	t.Helper()
	rows, err := pool.Query(t.Context(),
		`SELECT user_id::text, role, can_vote FROM cabal_members WHERE cabal_id = $1 ORDER BY joined_at`, c.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []memberRow
	for rows.Next() {
		var m memberRow
		if err := rows.Scan(&m.userID, &m.role, &m.canVote); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestNewCabal_defaultsToARequestCabalOfAllVotersWithOnlyItsCreator(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	var joinMode, voterMode, threshold string
	if err := pool.QueryRow(t.Context(), `SELECT join_mode, voter_mode, threshold FROM cabals WHERE id = $1`,
		c.ID.UUID()).Scan(&joinMode, &voterMode, &threshold); err != nil {
		t.Fatal(err)
	}
	if joinMode != "request" || voterMode != "all" || threshold != "majority" {
		t.Fatalf("cabal = %s %s %s, want request all majority", joinMode, voterMode, threshold)
	}
	want := []memberRow{{c.Creator.ID.String(), "creator", true}}
	if got := memberRows(t, pool, c); !slices.Equal(got, want) || len(c.Members) != 1 || c.Members[0] != c.Creator {
		t.Fatalf("members = %+v (fixture %+v), want %+v", got, c.Members, want)
	}
}

func TestNewCabal_seedsAnInviteCodeAndATreasuryWalletThatMatchTheFixture(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	if !regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{10}$`).MatchString(c.InviteCode) {
		t.Fatalf("invite code %q is not 10 Crockford base32 characters", c.InviteCode)
	}
	var code, wallet, address string
	if err := pool.QueryRow(t.Context(), `SELECT c.invite_code, w.privy_wallet_id, w.address
		FROM cabals c JOIN treasury_wallets w ON w.cabal_id = c.id WHERE c.id = $1`,
		c.ID.UUID()).Scan(&code, &wallet, &address); err != nil {
		t.Fatal(err)
	}
	if code != c.InviteCode || wallet != c.PrivyWalletID || address != string(c.TreasuryAddress) {
		t.Fatalf("stored %q, %q, %q; fixture says %q, %q, %q",
			code, wallet, address, c.InviteCode, c.PrivyWalletID, c.TreasuryAddress)
	}
}

func TestNewCabal_seedsMembersInJoinOrderWithTheCreatorFirstAndVotersByMode(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	for _, tt := range []struct {
		voterMode string
		canVote   []bool
	}{
		{"all", []bool{true, true, true}},
		{"list", []bool{true, false, false}},
	} {
		c := testkit.NewCabal(t, pool, testkit.WithMembers(3), testkit.WithVoterMode(tt.voterMode))
		want := make([]memberRow, 0, len(c.Members))
		for i, m := range c.Members {
			role := "member"
			if i == 0 {
				role = "creator"
			}
			want = append(want, memberRow{m.ID.String(), role, tt.canVote[i]})
		}
		if got := memberRows(t, pool, c); !slices.Equal(got, want) || c.Members[0] != c.Creator {
			t.Errorf("voter mode %s: members = %+v, want %+v with the creator first", tt.voterMode, got, want)
		}
	}
}

func TestNewCabal_storesTheJoinAndVoterModesItIsGiven(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool, testkit.WithJoinMode("request"), testkit.WithVoterMode("list"))
	var joinMode, voterMode string
	if err := pool.QueryRow(t.Context(), `SELECT join_mode, voter_mode FROM cabals WHERE id = $1`,
		c.ID.UUID()).Scan(&joinMode, &voterMode); err != nil || joinMode != "request" || voterMode != "list" {
		t.Fatalf("modes = %q, %q, %v; want request and list", joinMode, voterMode, err)
	}
}

func TestNewCabal_givesEachCabalItsOwnIDInviteCodeAndTreasury(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := testkit.NewCabal(t, pool), testkit.NewCabal(t, pool)
	if a.ID == b.ID || a.InviteCode == b.InviteCode || a.PrivyWalletID == b.PrivyWalletID ||
		a.TreasuryAddress == b.TreasuryAddress || a.Creator.ID == b.Creator.ID {
		t.Fatalf("two cabals share an id, code, wallet, address or creator: %+v and %+v", a, b)
	}
}
