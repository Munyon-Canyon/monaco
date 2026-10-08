package admin_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	adminapi "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/adminapi"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

const unknownMint = "So11111111111111111111111111111111111111112"

func (f lookupFixture) seedAsset(t *testing.T) market.Asset {
	t.Helper()
	a := marketfake.AAPLx()
	f.exec(t, `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name, issuer_tradable, company_key,
		first_seen_at, updated_at) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)`,
		a.ID.UUID(), a.Symbol, a.Mint.String(), int16(a.Decimals), string(a.Issuer), string(a.Kind), a.DisplayName,
		a.IssuerTradable, a.CompanyKey, f.clock.Now())
	return a
}

func (f lookupFixture) stake(t *testing.T, user, cabal uuid.UUID, units int64) {
	t.Helper()
	f.exec(t, `INSERT INTO user_positions (user_id, cabal_id, share_units, contributed_micros, withdrawn_micros,
		updated_at) VALUES ($1, $2, $3, 0, 0, $4)`, user, cabal, units, f.clock.Now())
}

func (f lookupFixture) holding(t *testing.T, cabal uuid.UUID, mint string, units int64) {
	t.Helper()
	f.exec(t, `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
		VALUES ($1, $2, $3, 1, $4)`, cabal, mint, units, f.clock.Now())
}

func (f lookupFixture) seedCabal(t *testing.T) testkit.SeededCabal {
	t.Helper()
	creator := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "alice"})
	joiner := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "bob"})
	c := testkit.NewCabal(t, f.pool, testkit.WithCreator(creator.ID), testkit.WithJoiner(joiner.ID),
		testkit.WithName("Tech bros"))
	f.stake(t, creator.ID.UUID(), c.ID.UUID(), 25_000_000)
	return c
}

func (f lookupFixture) cabal(t *testing.T, id string) adminapi.AdminCabal {
	t.Helper()
	w := adminRequest(t, f.h, "/v1/admin/cabals/"+id, "viewer")
	var body adminapi.AdminCabal
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || w.Code != http.StatusOK {
		t.Fatalf("GET cabal = %d %s (%v)", w.Code, w.Body, err)
	}
	return body
}

func TestAdminLookup_Cabal_MembersWithStake(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	body := f.cabal(t, c.ID.String())
	if len(body.Members) != 2 || body.MemberCount != 2 || body.Name != "Tech bros" ||
		body.Status != adminapi.AdminCabalStatusActive || body.CreatorId != c.Creator.ID.UUID() ||
		body.TreasuryAddress != string(c.TreasuryAddress) {
		t.Fatalf("body = %+v", body)
	}
}

func TestAdminLookup_Cabal_MemberRowsAndRules(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	body := f.cabal(t, f.seedCabal(t).ID.String())
	creator, joiner := body.Members[0], body.Members[1]
	if *creator.Handle != "alice" || creator.Role != "creator" || creator.ShareUnits != "25000000" || !creator.CanVote {
		t.Fatalf("creator = %+v", creator)
	}
	if *joiner.Handle != "bob" || joiner.ShareUnits != "0" {
		t.Fatalf("joiner = %+v", joiner)
	}
	if r := body.Rules; r.JoinMode != "request" || r.VoterMode != "all" || r.Threshold != "majority" {
		t.Fatalf("rules = %+v", r)
	}
}

func TestAdminLookup_Cabal_HoldingsTxnsAndActions(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	asset := f.seedAsset(t)
	f.holding(t, c.ID.UUID(), asset.Mint.String(), 300_000_000)
	f.holding(t, c.ID.UUID(), unknownMint, 5)
	txn := f.ids.NewV7()
	f.exec(t, `INSERT INTO cabal_txns (id, cabal_id, kind, status, created_at, seq)
		VALUES ($1, $2, 'fund', 'settled', $3, 1)`, txn, c.ID.UUID(), f.clock.Now())
	f.action(t, "cabal", c.ID.String())
	body := f.cabal(t, c.ID.String())
	if len(body.Positions) != 2 || len(body.RecentTxns) != 1 || len(body.RecentAdminActions) != 1 {
		t.Fatalf("body = %+v", body)
	}
	unknown, known := body.Positions[0], body.Positions[1]
	if *known.Symbol != "AAPLx" || known.Units != "300000000" || unknown.Symbol != nil {
		t.Fatalf("positions = %+v", body.Positions)
	}
	if got := body.RecentTxns[0]; got.Id != txn || got.Scope != adminapi.AdminTxnScopeCabal || got.Kind != "fund" {
		t.Fatalf("txn = %+v", got)
	}
}

func TestAdminLookup_Cabal_MissingIsNotFound(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	for _, path := range []string{"/v1/admin/cabals/" + f.ids.NewV7().String(), "/v1/admin/cabals/" + uuid.NewString()} {
		if got := f.get(t, path, http.StatusNotFound); got["code"] != string(errs.CodeCabalNotFound) {
			t.Errorf("GET %s = %v", path, got)
		}
	}
}

func TestAdminLookup_Cabal_ReadsHandlesInChunksOfTheIdentityLimit(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	bulk := make([]uuid.UUID, 500)
	for i := range bulk {
		bulk[i] = f.ids.NewV7()
	}
	f.exec(t, `INSERT INTO users (id, privy_user_id, login_provider, auth_state_changed_at, created_at, updated_at)
		SELECT id, 'did:privy:bulk-' || id::text, 'sms', $2, $2, $2 FROM unnest($1::uuid[]) AS id`,
		bulk, f.clock.Now())
	f.exec(t, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
		SELECT $1, id, 'member', true, $3 FROM unnest($2::uuid[]) AS id`, c.ID.UUID(), bulk, f.clock.Now())
	if body := f.cabal(t, c.ID.String()); len(body.Members) != 502 {
		t.Fatalf("members = %d", len(body.Members))
	}
}

func TestAdminLookup_Cabal_QueryCountDoesNotGrowWithMembers(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	f.holding(t, c.ID.UUID(), unknownMint, 1)
	read := func() {
		if w := adminRequest(t, f.h, "/v1/admin/cabals/"+c.ID.String(), "viewer"); w.Code != http.StatusOK {
			t.Fatalf("status = %d %s", w.Code, w.Body)
		}
	}
	testkit.AssertQueries(t, "admin GetAdminCabal", read)
	for range 3 {
		member := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
		f.exec(
			t,
			`INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at) VALUES ($1, $2, 'member', true, $3)`,
			c.ID.UUID(),
			member.ID.UUID(),
			f.clock.Now(),
		)
		f.stake(t, member.ID.UUID(), c.ID.UUID(), 1)
	}
	testkit.AssertQueries(t, "admin GetAdminCabal", read)
}

func TestAdminLookup_Cabal_ShowsTheLatestPotValueAndSharePrice(t *testing.T) {
	t.Parallel()
	f := newLookupFixture(t)
	c := f.seedCabal(t)
	other := testkit.NewCabal(t, f.pool)
	before := f.cabal(t, c.ID.String())
	if before.PotMicros != nil || before.NavPerShareMicros != nil || before.ValuedAt != nil {
		t.Fatalf("unvalued cabal = %+v, want null pot, share price and valuation time", before)
	}
	valued := f.clock.Now().Add(-time.Hour)
	for _, row := range []struct {
		cabal    uuid.UUID
		at       time.Time
		pot, nav int64
	}{
		{c.ID.UUID(), valued.Add(-time.Hour), 20_000_000, 900_000},
		{c.ID.UUID(), valued, 25_000_000, 1_000_000},
		{other.ID.UUID(), valued.Add(time.Minute), 99_000_000, 5_000_000},
	} {
		f.exec(t, `INSERT INTO cabal_value_snapshots (cabal_id, at, value_micros, nav_per_share_micros, total_shares)
			VALUES ($1, $2, $3, $4, 25)`, row.cabal, row.at, row.pot, row.nav)
	}
	got := f.cabal(t, c.ID.String())
	if got.PotMicros == nil || *got.PotMicros != "25000000" || got.NavPerShareMicros == nil ||
		*got.NavPerShareMicros != "1000000" || got.ValuedAt == nil || !got.ValuedAt.Equal(valued) {
		t.Fatalf("valued cabal = pot %v nav %v at %v, want 25000000, 1000000 at %v",
			got.PotMicros, got.NavPerShareMicros, got.ValuedAt, valued)
	}
}
