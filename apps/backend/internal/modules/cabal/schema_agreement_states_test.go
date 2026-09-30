package cabal_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/domain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSchemaStoresBothDirectionsAndEveryStatusAnInviteCanReach(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	c := testkit.NewCabal(t, pool)
	cabalID, creatorID := c.ID.UUID(), c.Creator.ID.UUID()
	applicant := testkit.SeedUser(t, pool, testkit.UserOpts{}).ID.UUID()
	wantNoError(
		t,
		"a pending request",
		insertRequest(
			t,
			pool,
			cabalID,
			applicant,
			creatorID,
			string(domain.DirectionRequest),
			string(domain.AccessPending),
		),
	)
	invitee := testkit.SeedUser(t, pool, testkit.UserOpts{}).ID.UUID()
	wantNoError(
		t,
		"a pending invite",
		insertRequest(
			t,
			pool,
			cabalID,
			invitee,
			creatorID,
			string(domain.DirectionInvite),
			string(domain.AccessPending),
		),
	)
	for _, status := range accessStatuses() {
		wantNoError(t, "an invite moved to "+string(status),
			run(t, pool, `UPDATE cabal_access_requests SET status = $1 WHERE cabal_id = $2 AND user_id = $3`,
				string(status), cabalID, invitee))
	}
}

func TestSchemaStoresEveryRoleTheDomainKnows(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	empty := insertBareCabal(t, f, "0000000002")
	for _, role := range []domain.Role{domain.RoleCreator, domain.RoleMember} {
		user := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
		wantNoError(
			t,
			"a "+string(role),
			run(t, f.pool, `INSERT INTO cabal_members (cabal_id, user_id, role, can_vote, joined_at)
			VALUES ($1, $2, $3, true, now())`, empty, user, string(role)),
		)
	}
}
