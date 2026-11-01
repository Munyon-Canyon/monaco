package cabal_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f createFixture) deleted(t *testing.T, user ids.UserID) {
	t.Helper()
	e := events.UserDeleted{V: 1, UserID: user.UUID(), At: f.clock.Now()}
	err := f.uow.Do(system(t), func(ctx context.Context, tx db.Tx) error {
		return adapters.Standing{}.Deleted(ctx, tx, e, f.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f createFixture) fails(t *testing.T, sql string, run func() error) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), sql); err != nil {
		t.Fatal(err)
	}
	if err := run(); err == nil {
		t.Errorf("handler after %q: err = nil, want a failure", sql)
	}
}

func (f createFixture) tryDeleted(user ids.UserID) error {
	e := events.UserDeleted{V: 1, UserID: user.UUID(), At: f.clock.Now()}
	return f.uow.Do(observability.WithActor(context.Background(), "system:consumer.cabal.standing"),
		func(ctx context.Context, tx db.Tx) error {
			return adapters.Standing{}.Deleted(ctx, tx, e, f.clock.Now())
		})
}

func (f createFixture) tryMoved(user ids.UserID, from, to string) error {
	e := events.UserAccountStatusChanged{V: 1, UserID: user.UUID(), From: from, To: to, At: f.clock.Now()}
	return f.uow.Do(observability.WithActor(context.Background(), "system:consumer.cabal.standing"),
		func(ctx context.Context, tx db.Tx) error {
			return adapters.Standing{}.StatusChanged(ctx, tx, e, f.clock.Now())
		})
}

func (f createFixture) moved(t *testing.T, user ids.UserID, from, to string) {
	t.Helper()
	e := events.UserAccountStatusChanged{V: 1, UserID: user.UUID(), From: from, To: to, At: f.clock.Now()}
	err := f.uow.Do(system(t), func(ctx context.Context, tx db.Tx) error {
		return adapters.Standing{}.StatusChanged(ctx, tx, e, f.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
}

func (f createFixture) voters(t *testing.T, c testkit.SeededCabal) []ids.UserID {
	t.Helper()
	got, err := adapters.NewQueries(f.pool).VoterSet(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f createFixture) admins(t *testing.T, c testkit.SeededCabal) []ids.UserID {
	t.Helper()
	got, err := adapters.NewQueries(f.pool).Admins(t.Context(), c.ID)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (f createFixture) roleOf(
	t *testing.T,
	c testkit.SeededCabal,
	user ids.UserID,
) (role string, canVote, suspended bool) {
	t.Helper()
	err := f.pool.QueryRow(t.Context(),
		`SELECT role, can_vote, suspended FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`,
		c.ID.UUID(), user.UUID()).Scan(&role, &canVote, &suspended)
	if err != nil {
		t.Fatal(err)
	}
	return role, canVote, suspended
}

func ofUsers(users ...testkit.SeededUser) []ids.UserID {
	out := make([]ids.UserID, 0, len(users))
	for _, u := range users {
		out = append(out, u.ID)
	}
	return out
}

func sameUsers(a, b []ids.UserID) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.SortFunc(a, func(x, y ids.UserID) int { return compareIDs(x.UUID(), y.UUID()) })
	slices.SortFunc(b, func(x, y ids.UserID) int { return compareIDs(x.UUID(), y.UUID()) })
	return slices.Equal(a, b)
}

func compareIDs(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) }

func TestStanding_deletedVoterLeavesAUnanimousElectorateOfTheOtherTwo(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE cabals SET threshold = 'unanimous' WHERE id = $1`,
		c.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	f.deleted(t, c.Members[2].ID)
	if got := f.voters(t, c); !sameUsers(got, ofUsers(c.Members[0], c.Members[1])) {
		t.Errorf("voters = %v, want the creator and the first member", got)
	}
	left := f.payloads(t, events.TypeCabalMemberLeft)
	if len(left) != 1 || left[0]["user_id"] != c.Members[2].ID.String() || left[0]["was_voter"] != true {
		t.Errorf("cabal.member_left = %v, want one voter leaving", left)
	}
	if updated := f.payloads(t, events.TypeCabalUpdated); len(updated) != 0 {
		t.Errorf("cabal.updated = %v, want none while an admin remains", updated)
	}
}

func TestStanding_deletedSoleMemberClosesTheCabalOnceAcrossRedelivery(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	f.deleted(t, c.Creator.ID)
	f.deleted(t, c.Creator.ID)
	if got := f.voters(t, c); len(got) != 0 {
		t.Errorf("voters = %v, want none", got)
	}
	if left := f.payloads(t, events.TypeCabalMemberLeft); len(left) != 1 {
		t.Errorf("cabal.member_left = %v, want exactly one across the redelivery", left)
	}
	if got := f.status(t, c); got != "banned" {
		t.Errorf("status = %s, want banned", got)
	}
	closed := f.payloads(t, events.TypeCabalBanned)
	if len(closed) != 1 || closed[0]["cabal_id"] != c.ID.String() ||
		closed[0]["reason"] != "The last member deleted their account" {
		t.Errorf("cabal.banned = %v, want one for the cabal across the redelivery", closed)
	}
}

func TestStanding_deletedMemberLeavesACabalWithMembersOpen(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	f.deleted(t, c.Members[1].ID)
	if got := f.status(t, c); got != "active" {
		t.Errorf("status = %s, want active while a member remains", got)
	}
	if closed := f.payloads(t, events.TypeCabalBanned); len(closed) != 0 {
		t.Errorf("cabal.banned = %v, want none", closed)
	}
	f.deleted(t, c.Creator.ID)
	if got := f.status(t, c); got != "banned" {
		t.Errorf("status = %s, want banned once the last member is gone", got)
	}
}

func TestStanding_deletedSoleMemberOfABannedCabalAddsNoSecondBan(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool)
	if _, err := f.pool.Exec(
		t.Context(),
		`UPDATE cabals SET status = 'banned' WHERE id = $1`,
		c.ID.UUID(),
	); err != nil {
		t.Fatal(err)
	}
	f.deleted(t, c.Creator.ID)
	if closed := f.payloads(t, events.TypeCabalBanned); len(closed) != 0 {
		t.Errorf("cabal.banned = %v, want none for a cabal that was already banned", closed)
	}
}

func TestStanding_failsWhenTheEmptyCabalCannotBeClosed(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{
		"lock":  `ALTER TABLE cabals RENAME TO cabals_gone`,
		"close": `ALTER TABLE cabals RENAME COLUMN status TO status_gone`,
		"event": `ALTER TABLE events ADD CONSTRAINT no_ban CHECK (type <> 'cabal.banned')`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newCreate(t)
			c := testkit.NewCabal(t, f.pool)
			f.fails(t, sql, func() error { return f.tryDeleted(c.Creator.ID) })
		})
	}
}

func TestJoinCabal_refusesACabalClosedByItsLastMemberDeleting(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	e := events.UserDeleted{V: 1, UserID: c.Creator.ID.UUID(), At: f.clock.Now()}
	err := f.uow.Do(system(t), func(ctx context.Context, tx db.Tx) error {
		return adapters.Standing{}.Deleted(ctx, tx, e, f.clock.Now())
	})
	if err != nil {
		t.Fatal(err)
	}
	wantErr(t, f.join(t.Context(), f.user(t), c.ID), errs.CodeCabalBanned)
	rows, err := sqlc.New(f.pool).SearchCabals(t.Context(), sqlc.SearchCabalsParams{
		ActorID: f.user(t).UUID(), PageSize: 20,
	})
	if err != nil || len(rows) != 0 {
		t.Errorf("SearchCabals = %d rows, %v, want the closed cabal missing from the list", len(rows), err)
	}
}

func TestStanding_deletedLoneAdminHandsAdminToTheLongestStandingMember(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.deleted(t, c.Creator.ID)
	if got := f.admins(t, c); !sameUsers(got, ofUsers(c.Members[1])) {
		t.Errorf("admins = %v, want the longest-standing member", got)
	}
	f.deleted(t, c.Members[1].ID)
	if got := f.admins(t, c); !sameUsers(got, ofUsers(c.Members[2])) {
		t.Errorf("admins = %v, want the last member", got)
	}
}

func TestStanding_bannedCreatorIsDemotedAndTheLongestStandingMemberBecomesAdmin(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.moved(t, c.Creator.ID, "active", "banned")
	f.moved(t, c.Creator.ID, "active", "banned")

	if role, canVote, suspended := f.roleOf(t, c, c.Creator.ID); role != "member" || canVote || !suspended {
		t.Errorf(
			"banned creator = %s vote=%t suspended=%t, want a suspended non-voting member",
			role,
			canVote,
			suspended,
		)
	}
	if role, canVote, _ := f.roleOf(t, c, c.Members[1].ID); role != "admin" || !canVote {
		t.Errorf("longest-standing member = %s vote=%t, want an admin who votes", role, canVote)
	}
	if got := f.admins(t, c); !sameUsers(got, ofUsers(c.Members[1])) {
		t.Errorf("admins = %v, want only the longest-standing member", got)
	}
	if got := f.voters(t, c); !sameUsers(got, ofUsers(c.Members[1], c.Members[2])) {
		t.Errorf("voters = %v, want the two active members", got)
	}
	updated := f.payloads(t, events.TypeCabalUpdated)
	if len(updated) != 1 || updated[0]["actor_id"] != c.Members[1].ID.String() {
		t.Errorf("cabal.updated = %v, want one admin change by the promoted member", updated)
	}
}

func TestStanding_bannedMemberStaysOutOfEveryLaterElectorate(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.moved(t, c.Members[2].ID, "active", "banned")
	if updated := f.payloads(t, events.TypeCabalUpdated); len(updated) != 0 {
		t.Errorf("cabal.updated = %v, want none while an admin remains", updated)
	}
	q := sqlc.New(f.pool)
	if err := q.SetAllMembersVote(t.Context(), c.ID.UUID()); err != nil {
		t.Fatal(err)
	}
	if got := f.voters(t, c); !sameUsers(got, ofUsers(c.Members[0], c.Members[1])) {
		t.Errorf("voters after SetAllMembersVote = %v, want the two active members", got)
	}
	err := q.SetMemberVoters(t.Context(), sqlc.SetMemberVotersParams{
		CabalID: c.ID.UUID(), VoterIds: []uuid.UUID{c.Members[1].ID.UUID(), c.Members[2].ID.UUID()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := f.voters(t, c); !sameUsers(got, ofUsers(c.Members[0], c.Members[1])) {
		t.Errorf("voters after SetMemberVoters = %v, want the two active members", got)
	}
	n, err := q.SetMemberRole(t.Context(), sqlc.SetMemberRoleParams{
		Role: "admin", CanVote: true, CabalID: c.ID.UUID(), UserID: c.Members[2].ID.UUID(),
	})
	if err != nil || n != 0 {
		t.Errorf("SetMemberRole on the banned member = %d, %v, want no row", n, err)
	}
}

func TestStanding_adminSearchSkipsMembersBannedEarlier(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.moved(t, c.Members[1].ID, "active", "banned")
	f.moved(t, c.Creator.ID, "active", "banned")
	if got := f.admins(t, c); !sameUsers(got, ofUsers(c.Members[2])) {
		t.Errorf("admins = %v, want the one member who is not banned", got)
	}
}

func TestStanding_cabalWithOnlyBannedMembersHasNoOneToPromote(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	f.moved(t, c.Members[1].ID, "active", "banned")
	f.moved(t, c.Creator.ID, "active", "banned")
	if got := f.admins(t, c); len(got) != 0 {
		t.Errorf("admins = %v, want none", got)
	}
}

func TestStanding_liftedBanRestoresTheVoteButNotTheRole(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	f.moved(t, c.Creator.ID, "active", "banned")
	f.moved(t, c.Creator.ID, "banned", "active")
	if role, canVote, suspended := f.roleOf(t, c, c.Creator.ID); role != "member" || !canVote || suspended {
		t.Errorf("restored creator = %s vote=%t suspended=%t, want a voting member", role, canVote, suspended)
	}
	if got := f.admins(t, c); !sameUsers(got, ofUsers(c.Members[1])) {
		t.Errorf("admins = %v, want the promoted member only", got)
	}
}

func TestStanding_liftedBanKeepsAListCabalMemberOffTheVote(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3), testkit.WithVoterMode("list"))
	f.moved(t, c.Members[2].ID, "active", "banned")
	f.moved(t, c.Members[2].ID, "banned", "active")
	if _, canVote, suspended := f.roleOf(t, c, c.Members[2].ID); canVote || suspended {
		t.Errorf("restored member vote=%t suspended=%t, want a non-voting active member", canVote, suspended)
	}
}

func TestStanding_ignoresStatusChangesThatDoNotInvolveABan(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	f.moved(t, c.Members[1].ID, "active", "suspended")
	f.moved(t, c.Members[1].ID, "suspended", "active")
	if got := f.voters(t, c); len(got) != 2 {
		t.Errorf("voters = %v, want both members", got)
	}
}

func TestStanding_failsWhenTheMembershipsCannotBeWritten(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		sql  string
		run  func(f createFixture, c testkit.SeededCabal) error
	}{
		{
			"delete",
			`ALTER TABLE cabal_members RENAME COLUMN can_vote TO vote_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryDeleted(c.Members[2].ID)
			},
		},
		{
			"deleted event",
			`ALTER TABLE events RENAME TO events_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryDeleted(c.Members[2].ID)
			},
		},
		{
			"suspend",
			`ALTER TABLE cabal_members RENAME COLUMN suspended TO suspended_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryMoved(c.Members[2].ID, "active", "banned")
			},
		},
		{
			"restore",
			`ALTER TABLE cabal_members RENAME COLUMN suspended TO suspended_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryMoved(c.Members[2].ID, "banned", "active")
			},
		},
		{
			"promote",
			`ALTER TABLE cabal_members RENAME COLUMN joined_at TO joined_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryMoved(c.Creator.ID, "active", "banned")
			},
		},
		{
			"promoted event",
			`ALTER TABLE events RENAME TO events_gone`,
			func(f createFixture, c testkit.SeededCabal) error {
				return f.tryMoved(c.Creator.ID, "active", "banned")
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := newCreate(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
			f.fails(t, tc.sql, func() error { return tc.run(f, c) })
		})
	}
}
