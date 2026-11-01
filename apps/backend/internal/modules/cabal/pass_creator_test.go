package cabal_test

import (
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) storedCreator(t *testing.T, c testkit.SeededCabal) (uuid.UUID, int) {
	t.Helper()
	var creator uuid.UUID
	var creators int
	if err := f.pool.QueryRow(t.Context(), `SELECT creator_id,
		(SELECT count(*) FROM cabal_members WHERE cabal_id = $1 AND role = 'creator')::int
		FROM cabals WHERE id = $1`, c.ID.UUID()).Scan(&creator, &creators); err != nil {
		t.Fatal(err)
	}
	return creator, creators
}

func TestApplyRuleChange_creatorHandoverSwapsTheRolesOnceAndLetsTheFormerCreatorLeave(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	next := c.Members[1]
	f.makeAdmin(t, c.ID, next.ID)
	user := next.ID.UUID()
	e := rulePassed(c, c.Creator.ID, boolChange("creator", true, &user))
	for range 2 {
		if err := f.deliver(t.Context(), e); err != nil {
			t.Fatal(err)
		}
	}
	if role, vote := f.role(t, c, next.ID); role != "creator" || !vote {
		t.Fatalf("new creator role = %s vote = %v, want a voting creator", role, vote)
	}
	if role, vote := f.role(t, c, c.Creator.ID); role != "admin" || !vote {
		t.Fatalf("old creator role = %s vote = %v, want a voting admin", role, vote)
	}
	if stored, creators := f.storedCreator(t, c); stored != user || creators != 1 {
		t.Fatalf("cabals.creator_id = %s with %d creator rows, want %s and one", stored, creators, user)
	}
	f.wantHandoverAnnounced(t, c, e, user)
	if err := f.leave(t.Context(), &treasuryStub{}, c.Creator.ID, c.ID); err != nil {
		t.Fatalf("former creator leave: %v, want it to leave like any member", err)
	}
	if _, ok := f.membership(t, c.ID, c.Creator.ID); ok {
		t.Fatal("the former creator is still a member")
	}
}

func (f accessFixture) wantHandoverAnnounced(
	t *testing.T, c testkit.SeededCabal, e events.ProposalRulePassed, next uuid.UUID,
) {
	t.Helper()
	updates := f.updates(t)
	if len(updates) != 1 || updates[0].ActorID != c.Creator.ID.UUID() || updates[0].Changes.CreatorID == nil ||
		*updates[0].Changes.CreatorID != next || *updates[0].Changes.ProposalID != e.ProposalID {
		t.Fatalf("cabal.updated = %+v, want one by the old creator naming the new one", updates)
	}
}

func TestApplyRuleChange_creatorHandoverIsSkippedWhenItNoLongerHolds(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(4))
	admin, member, gone := c.Members[1].ID, c.Members[2].ID, uuid.NewSHA1(uuid.NameSpaceOID, []byte("gone"))
	f.makeAdmin(t, c.ID, admin)
	adminID, memberID := admin.UUID(), member.UUID()
	for name, e := range map[string]struct {
		proposer ids.UserID
		change   string
	}{
		"a target who lost the admin role":  {c.Creator.ID, boolChange("creator", true, &memberID)},
		"a target who left":                 {c.Creator.ID, boolChange("creator", true, &gone)},
		"a proposer who is not the creator": {c.Members[3].ID, boolChange("creator", true, &adminID)},
		"a change that is not a handover":   {c.Creator.ID, boolChange("creator", false, &adminID)},
	} {
		if err := f.deliver(t.Context(), rulePassed(c, e.proposer, e.change)); err != nil {
			t.Errorf("%s: %v, want an ack", name, err)
		}
	}
	if stored, creators := f.storedCreator(t, c); stored != c.Creator.ID.UUID() || creators != 1 {
		t.Fatalf("creator_id = %s with %d creator rows, want the original creator only", stored, creators)
	}
	if role, _ := f.role(t, c, c.Creator.ID); role != "creator" || len(f.updates(t)) != 0 {
		t.Fatalf("creator role = %s, events %+v; want nothing written", role, f.updates(t))
	}
}

func TestApplyRuleChange_creatorHandoverWrapsStoreFailuresAsInternal(t *testing.T) {
	t.Parallel()
	for name, statement := range map[string]string{
		"the creator write": `ALTER TABLE cabals ADD CONSTRAINT keep_creator CHECK (creator_id = '%s') NOT VALID`,
		"the demotion":      `ALTER TABLE cabal_members ADD CONSTRAINT no_admin CHECK (role <> 'admin') NOT VALID`,
		"the promotion":     `ALTER TABLE cabal_members ADD CONSTRAINT no_creator CHECK (role <> 'creator') NOT VALID`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
			f.makeAdmin(t, c.ID, c.Members[1].ID)
			user := c.Members[1].ID.UUID()
			f.exec(t, strings.ReplaceAll(statement, "%s", c.Creator.ID.String()))
			wantErr(t, f.deliver(t.Context(), rulePassed(c, c.Creator.ID, boolChange("creator", true, &user))),
				errs.CodeInternal)
		})
	}
}
