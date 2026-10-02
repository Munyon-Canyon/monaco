package cabal_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type queriesFixture struct {
	pool  *pgxpool.Pool
	q     *sqlc.Queries
	clock *testkit.Clock
	uow   *db.UnitOfWork
}

func newQueries(t *testing.T) queriesFixture {
	t.Helper()
	pool := testkit.DB(t)
	c := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	return queriesFixture{
		pool:  pool,
		q:     sqlc.New(pool),
		clock: c,
		uow:   db.New(pool, testkit.NewIDs(testkit.RandSeed(t)), c),
	}
}

func wantNoRows(t *testing.T, what string, err error) {
	t.Helper()
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Errorf("%s: err = %v, want no rows", what, err)
	}
}

func wantRows(t *testing.T, what string, got, want int64, err error) {
	t.Helper()
	if err != nil || got != want {
		t.Errorf("%s: %d rows, %v; want %d", what, got, err, want)
	}
}

func sameInstant(t *testing.T, what string, got, want time.Time) {
	t.Helper()
	if !got.Equal(want) {
		t.Errorf("%s = %s, want %s", what, got, want)
	}
}

func idsOf[T any](rows []T, id func(T) uuid.UUID) []uuid.UUID {
	out := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		out[i] = id(r)
	}
	return out
}

func wantIDs(t *testing.T, what string, got []uuid.UUID, err error, want ...uuid.UUID) {
	t.Helper()
	if err != nil || !slices.Equal(got, want) {
		t.Errorf("%s = %v, %v; want %v", what, got, err, want)
	}
}

func insertMember(
	t *testing.T,
	f queriesFixture,
	cabalID, userID uuid.UUID,
	role string,
	canVote bool,
	at time.Time,
) int64 {
	t.Helper()
	n, err := f.q.InsertMember(t.Context(), sqlc.InsertMemberParams{
		CabalID: cabalID, UserID: userID, Role: role, CanVote: canVote, JoinedAt: at,
	})
	if err != nil {
		t.Fatalf("InsertMember: %v", err)
	}
	return n
}

func newCabalParams(creatorID uuid.UUID, inviteCode string, now time.Time) sqlc.InsertCabalParams {
	return sqlc.InsertCabalParams{
		ID: newID(), Name: "Friends pot", CreatorID: creatorID, JoinMode: "open", VoterMode: "all",
		Threshold: "majority", ProposalExpirySeconds: 86400, SlippageBps: 100, InviteCode: inviteCode, Now: now,
	}
}

func TestCabalQueries_insertThenFindReturnsTheCabalAndCountsItsMembers(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	creator := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	id := newID()
	n, err := f.q.InsertCabal(t.Context(), sqlc.InsertCabalParams{
		ID: id, Name: "Friends pot", CreatorID: creator.ID.UUID(), JoinMode: "request", VoterMode: "list",
		Threshold: "unanimous", ProposalExpirySeconds: 604800, SlippageBps: 50, InviteCode: "ABCDEFGHJK",
		Now: f.clock.Now(),
	})
	wantRows(t, "InsertCabal", n, 1, err)
	got, err := f.q.FindCabal(t.Context(), id)
	if err != nil {
		t.Fatalf("FindCabal: %v", err)
	}
	created, updated := got.CreatedAt, got.UpdatedAt
	got.CreatedAt, got.UpdatedAt = time.Time{}, time.Time{}
	want := sqlc.FindCabalRow{
		ID: id, Name: "Friends pot", CreatorID: creator.ID.UUID(), JoinMode: "request", VoterMode: "list",
		Threshold: "unanimous", ProposalExpirySeconds: 604800, SlippageBps: 50, InviteCode: "ABCDEFGHJK",
		Status: "active",
	}
	if got != want {
		t.Fatalf("FindCabal = %+v, want %+v", got, want)
	}
	sameInstant(t, "created_at", created, f.clock.Now())
	sameInstant(t, "updated_at", updated, f.clock.Now())
	insertMember(t, f, id, creator.ID.UUID(), "creator", true, f.clock.Now())
	insertMember(t, f, id, testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID(), "member", false, f.clock.Now())
	if got, err := f.q.FindCabal(t.Context(), id); err != nil || got.MemberCount != 2 {
		t.Fatalf("member count = %d, %v; want 2", got.MemberCount, err)
	}
	_, err = f.q.FindCabal(t.Context(), newID())
	wantNoRows(t, "FindCabal on an unknown id", err)
}

func TestCabalQueries_findByInviteCodeReadsTheCabalAndMissesAnUnknownCode(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	a := testkit.NewCabal(t, f.pool, testkit.WithMembers(3), testkit.WithJoinMode("request"))
	testkit.NewCabal(t, f.pool)
	got, err := f.q.FindCabalByInviteCode(t.Context(), a.InviteCode)
	if err != nil || got.ID != a.ID.UUID() || got.MemberCount != 3 || got.JoinMode != "request" {
		t.Fatalf("FindCabalByInviteCode = %+v, %v; want cabal a with 3 members", got, err)
	}
	_, err = f.q.FindCabalByInviteCode(t.Context(), "ZZZZZZZZZZ")
	wantNoRows(t, "FindCabalByInviteCode on an unknown code", err)
}

func TestCabalQueries_listReturnsTheKnownCabalsOldestFirstWithTheirMemberCounts(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	a := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	b := testkit.NewCabal(t, f.pool)
	listed, err := f.q.ListCabals(t.Context(), []uuid.UUID{b.ID.UUID(), newID(), a.ID.UUID()})
	wantIDs(
		t,
		"ListCabals",
		idsOf(listed, func(r sqlc.ListCabalsRow) uuid.UUID { return r.ID }),
		err,
		a.ID.UUID(),
		b.ID.UUID(),
	)
	counts := make([]int32, len(listed))
	for i, r := range listed {
		counts[i] = r.MemberCount
	}
	if !slices.Equal(counts, []int32{3, 1}) {
		t.Errorf("member counts = %v, want 3 then 1", counts)
	}
	none, err := f.q.ListCabals(t.Context(), nil)
	wantIDs(t, "ListCabals with no ids", idsOf(none, func(r sqlc.ListCabalsRow) uuid.UUID { return r.ID }), err)
}

func TestCabalQueries_updateRewritesTheEditableFieldsAndBumpsUpdatedAt(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	before, err := f.q.FindCabal(t.Context(), c.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(time.Minute)
	n, err := f.q.UpdateCabal(t.Context(), sqlc.UpdateCabalParams{
		ID: c.ID.UUID(), Name: "Work pot", JoinMode: "request", VoterMode: "list", Threshold: "unanimous",
		ProposalExpirySeconds: 3600, SlippageBps: 250, Now: f.clock.Now(),
	})
	wantRows(t, "UpdateCabal", n, 1, err)
	after, err := f.q.FindCabal(t.Context(), c.ID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	sameInstant(t, "updated_at", after.UpdatedAt, f.clock.Now())
	sameInstant(t, "created_at", after.CreatedAt, before.CreatedAt)
	after.UpdatedAt, after.CreatedAt, before.UpdatedAt, before.CreatedAt = time.Time{}, time.Time{}, time.Time{}, time.Time{}
	before.Name, before.JoinMode, before.VoterMode, before.Threshold = "Work pot", "request", "list", "unanimous"
	before.ProposalExpirySeconds, before.SlippageBps = 3600, 250
	if after != before {
		t.Fatalf("after the update %+v, want only the editable fields changed: %+v", after, before)
	}
	n, err = f.q.UpdateCabal(t.Context(), sqlc.UpdateCabalParams{
		ID: newID(), Name: "Ghost", JoinMode: "open", VoterMode: "all", Threshold: "majority",
		ProposalExpirySeconds: 3600, SlippageBps: 100, Now: f.clock.Now(),
	})
	wantRows(t, "UpdateCabal on an unknown id", n, 0, err)
}

func TestCabalQueries_setPictureSetsThenClears(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	url := pgtype.Text{String: "https://cdn.example/cabals/c.png", Valid: true}
	for _, step := range []struct {
		picture pgtype.Text
		want    pgtype.Text
	}{{url, url}, {pgtype.Text{}, pgtype.Text{}}} {
		f.clock.Advance(time.Minute)
		n, err := f.q.SetCabalPicture(t.Context(), sqlc.SetCabalPictureParams{
			ID: c.ID.UUID(), PictureUrl: step.picture, Now: f.clock.Now(),
		})
		wantRows(t, "SetCabalPicture", n, 1, err)
		got, err := f.q.FindCabal(t.Context(), c.ID.UUID())
		if err != nil || got.PictureUrl != step.want {
			t.Fatalf("picture = %+v, %v; want %+v", got.PictureUrl, err, step.want)
		}
		sameInstant(t, "updated_at", got.UpdatedAt, f.clock.Now())
	}
	n, err := f.q.SetCabalPicture(t.Context(), sqlc.SetCabalPictureParams{ID: newID(), Now: f.clock.Now()})
	wantRows(t, "SetCabalPicture on an unknown id", n, 0, err)
}

func TestCabalQueries_anInviteCodeClashInsertsNothingAndLeavesTheTransactionUsable(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	taken := testkit.NewCabal(t, f.pool)
	creator := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		clash, err := q.InsertCabal(ctx, newCabalParams(creator, taken.InviteCode, f.clock.Now()))
		wantRows(t, "InsertCabal with a taken invite code", clash, 0, err)
		retry, err := q.InsertCabal(ctx, newCabalParams(creator, "0123456789", f.clock.Now()))
		wantRows(t, "InsertCabal again with a fresh code in the same transaction", retry, 1, err)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var cabals int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabals`).Scan(&cabals); err != nil || cabals != 2 {
		t.Fatalf("cabals = %d, %v; want the taken one and the retry", cabals, err)
	}
}

func tryCabalLock(ctx context.Context, t *testing.T, f queriesFixture, cabalID uuid.UUID, mode string) error {
	t.Helper()
	return f.pool.QueryRow(ctx, `SELECT 1 FROM cabals WHERE id = $1 FOR `+mode+` NOWAIT`, cabalID).Scan(new(int))
}

func wantLockRefused(t *testing.T, what string, err error) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != "55P03" {
		t.Errorf("%s: %v, want lock_not_available", what, err)
	}
}

func TestCabalQueries_aSharedLockAdmitsOtherSharersAndRefusesAnUpdater(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		locked, err := sqlc.New(tx.Queries()).LockCabalShared(ctx, c.ID.UUID())
		if err != nil || locked != c.ID.UUID() {
			t.Errorf("LockCabalShared = %s, %v; want the cabal id", locked, err)
		}
		if err := tryCabalLock(ctx, t, f, c.ID.UUID(), "SHARE"); err != nil {
			t.Errorf("a second sharer got %v, want it admitted", err)
		}
		wantLockRefused(t, "an updater under a shared lock", tryCabalLock(ctx, t, f, c.ID.UUID(), "UPDATE"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tryCabalLock(t.Context(), t, f, c.ID.UUID(), "UPDATE"); err != nil {
		t.Errorf("after the transaction: %v, want the row unlocked", err)
	}
}

func TestCabalQueries_anExclusiveLockRefusesEveryOtherLockUntilTheTransactionEnds(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		locked, err := sqlc.New(tx.Queries()).LockCabalExclusive(ctx, c.ID.UUID())
		if err != nil || locked != c.ID.UUID() {
			t.Errorf("LockCabalExclusive = %s, %v; want the cabal id", locked, err)
		}
		wantLockRefused(t, "a sharer under an exclusive lock", tryCabalLock(ctx, t, f, c.ID.UUID(), "SHARE"))
		wantLockRefused(t, "an updater under an exclusive lock", tryCabalLock(ctx, t, f, c.ID.UUID(), "UPDATE"))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tryCabalLock(t.Context(), t, f, c.ID.UUID(), "SHARE"); err != nil {
		t.Errorf("after the transaction: %v, want the row unlocked", err)
	}
}

func TestCabalQueries_anEditLockAdmitsAMemberInsert(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	newcomer := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		locked, err := sqlc.New(tx.Queries()).LockCabalExclusive(ctx, c.ID.UUID())
		if err != nil || locked != c.ID.UUID() {
			t.Errorf("LockCabalExclusive = %s, %v; want the cabal id", locked, err)
		}
		conn, err := f.pool.Acquire(ctx)
		if err != nil {
			return err
		}
		defer conn.Release()
		if _, err := conn.Exec(ctx, `SET lock_timeout = '250ms'`); err != nil {
			return err
		}
		defer func() { _, _ = conn.Exec(ctx, `RESET lock_timeout`) }()
		n, err := sqlc.New(conn).InsertMember(ctx, sqlc.InsertMemberParams{
			CabalID: c.ID.UUID(), UserID: newcomer.ID.UUID(), Role: "member", CanVote: false, JoinedAt: f.clock.Now(),
		})
		if err != nil || n != 1 {
			t.Errorf("InsertMember under an edit lock = %d, %v; want one row", n, err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestCabalQueries_lockingAnUnknownCabalFindsNoRow(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	_, err := f.q.LockCabalShared(t.Context(), newID())
	wantNoRows(t, "LockCabalShared on an unknown id", err)
	_, err = f.q.LockCabalExclusive(t.Context(), newID())
	wantNoRows(t, "LockCabalExclusive on an unknown id", err)
}

func TestMemberQueries_insertKeepsTheFirstRowAndFindReadsItBack(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	newcomer := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	cabalID, userID := c.ID.UUID(), newcomer.ID.UUID()
	wantRows(t, "first insert", insertMember(t, f, cabalID, userID, "member", true, f.clock.Now()), 1, nil)
	wantRows(
		t,
		"second insert",
		insertMember(t, f, cabalID, userID, "member", false, f.clock.Now().Add(time.Hour)),
		0,
		nil,
	)
	got, err := f.q.FindMember(t.Context(), sqlc.FindMemberParams{CabalID: cabalID, UserID: userID})
	if err != nil || got.Role != "member" || !got.CanVote {
		t.Fatalf("FindMember = %+v, %v; want the first insert's member who can vote", got, err)
	}
	sameInstant(t, "joined_at", got.JoinedAt, f.clock.Now())
	stranger := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	_, err = f.q.FindMember(t.Context(), sqlc.FindMemberParams{CabalID: cabalID, UserID: stranger.ID.UUID()})
	wantNoRows(t, "FindMember on a stranger", err)
}

func TestMemberQueries_listsFollowJoinTimeNotIDAndVoterQueriesReadCanVote(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(4), testkit.WithVoterMode("list"))
	cabalID := c.ID.UUID()
	creator, err := f.q.FindMember(t.Context(), sqlc.FindMemberParams{CabalID: cabalID, UserID: c.Creator.ID.UUID()})
	if err != nil {
		t.Fatal(err)
	}
	early := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	insertMember(t, f, cabalID, early.ID.UUID(), "member", true, creator.JoinedAt.Add(-time.Hour))
	if err := f.q.SetMemberVoters(t.Context(), sqlc.SetMemberVotersParams{
		CabalID: cabalID, VoterIds: []uuid.UUID{early.ID.UUID(), c.Members[2].ID.UUID()},
	}); err != nil {
		t.Fatal(err)
	}
	wantOrder := []testkit.SeededUser{early, c.Members[0], c.Members[1], c.Members[2], c.Members[3]}
	wantVoter := []bool{true, true, false, true, false}
	members, err := f.q.ListMembers(t.Context(), cabalID)
	if err != nil || len(members) != len(wantOrder) {
		t.Fatalf("ListMembers = %+v, %v; want %d members", members, err, len(wantOrder))
	}
	for i, m := range members {
		wantRole := "member"
		if i == 1 {
			wantRole = "creator"
		}
		if m.UserID != wantOrder[i].ID.UUID() || m.Role != wantRole || m.CanVote != wantVoter[i] {
			t.Errorf("member %d = %+v, want %s (%s, can vote %t)", i, m, wantOrder[i].ID, wantRole, wantVoter[i])
		}
	}
	voters, err := f.q.ListVoterIDs(t.Context(), cabalID)
	want := []uuid.UUID{early.ID.UUID(), c.Members[0].ID.UUID(), c.Members[2].ID.UUID()}
	if err != nil || !slices.Equal(voters, want) {
		t.Fatalf("ListVoterIDs = %v, %v; want %v", voters, err, want)
	}
}

func TestMemberQueries_cabalIDsForUserListEveryCabalTheyAreIn(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	first := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	second := testkit.NewCabal(t, f.pool)
	other := testkit.NewCabal(t, f.pool)
	member := first.Members[1].ID.UUID()
	joined, err := f.q.FindMember(t.Context(), sqlc.FindMemberParams{CabalID: first.ID.UUID(), UserID: member})
	if err != nil {
		t.Fatal(err)
	}
	insertMember(t, f, second.ID.UUID(), member, "member", true, joined.JoinedAt.Add(-time.Hour))
	got, err := f.q.ListCabalIDsForUser(t.Context(), member)
	if want := []uuid.UUID{second.ID.UUID(), first.ID.UUID()}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("ListCabalIDsForUser = %v, %v; want %v and not %s", got, err, want, other.ID)
	}
	if none, err := f.q.ListCabalIDsForUser(t.Context(), newID()); err != nil || len(none) != 0 {
		t.Fatalf("ListCabalIDsForUser for a stranger = %v, %v; want none", none, err)
	}
}

func TestMemberQueries_votersAreSetPerCabalAndTheCreatorAlwaysStays(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(4), testkit.WithVoterMode("list"))
	bystander := testkit.NewCabal(t, f.pool, testkit.WithMembers(2), testkit.WithVoterMode("list"))
	voters := func(cabal testkit.SeededCabal) []bool {
		t.Helper()
		members, err := f.q.ListMembers(t.Context(), cabal.ID.UUID())
		if err != nil {
			t.Fatal(err)
		}
		flags := make([]bool, len(members))
		for i, m := range members {
			flags[i] = m.CanVote
		}
		return flags
	}
	steps := []struct {
		name string
		do   func() error
		want []bool
	}{
		{"voters 1 and 3", func() error {
			return f.q.SetMemberVoters(t.Context(), sqlc.SetMemberVotersParams{
				CabalID: c.ID.UUID(), VoterIds: []uuid.UUID{c.Members[1].ID.UUID(), c.Members[3].ID.UUID()},
			})
		}, []bool{true, true, false, true}},
		{"an empty list", func() error {
			return f.q.SetMemberVoters(t.Context(), sqlc.SetMemberVotersParams{CabalID: c.ID.UUID()})
		}, []bool{true, false, false, false}},
		{
			"everyone",
			func() error { return f.q.SetAllMembersVote(t.Context(), c.ID.UUID()) },
			[]bool{true, true, true, true},
		},
	}
	for _, s := range steps {
		if err := s.do(); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
		if got := voters(c); !slices.Equal(got, s.want) {
			t.Errorf("after %s: can_vote = %v, want %v", s.name, got, s.want)
		}
		if got := voters(bystander); !slices.Equal(got, []bool{true, false}) {
			t.Errorf("after %s: another cabal's can_vote = %v, want it untouched", s.name, got)
		}
	}
}

func TestMemberQueries_deleteReturnsTheRowItRemovedAndOnlyThatRow(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3), testkit.WithVoterMode("list"))
	other := testkit.NewCabal(t, f.pool)
	leaver := c.Members[1].ID.UUID()
	insertMember(t, f, other.ID.UUID(), leaver, "member", true, f.clock.Now())
	gone, err := f.q.DeleteMember(t.Context(), sqlc.DeleteMemberParams{CabalID: c.ID.UUID(), UserID: leaver})
	if err != nil || gone.Role != "member" || gone.CanVote {
		t.Fatalf("DeleteMember = %+v, %v; want the member who could not vote", gone, err)
	}
	_, err = f.q.DeleteMember(t.Context(), sqlc.DeleteMemberParams{CabalID: c.ID.UUID(), UserID: leaver})
	wantNoRows(t, "a second DeleteMember", err)
	if left, err := f.q.ListMembers(t.Context(), c.ID.UUID()); err != nil || len(left) != 2 {
		t.Fatalf("members left = %+v, %v; want 2", left, err)
	}
	if elsewhere, err := f.q.ListCabalIDsForUser(
		t.Context(),
		leaver,
	); err != nil ||
		!slices.Equal(elsewhere, []uuid.UUID{other.ID.UUID()}) {
		t.Fatalf("cabals of the leaver = %v, %v; want only the other cabal", elsewhere, err)
	}
	gone, err = f.q.DeleteMember(
		t.Context(),
		sqlc.DeleteMemberParams{CabalID: c.ID.UUID(), UserID: c.Creator.ID.UUID()},
	)
	if err != nil || gone.Role != "creator" || !gone.CanVote {
		t.Fatalf("DeleteMember of the creator = %+v, %v; want the creator who could vote", gone, err)
	}
}

func TestMemberQueries_lockMembersListsEveryMemberAndHoldsTheirRowsUntilTheTransactionEnds(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	tryLock := func() error {
		return f.pool.QueryRow(t.Context(),
			`SELECT 1 FROM cabal_members WHERE cabal_id = $1 FOR UPDATE NOWAIT`, c.ID.UUID()).Scan(new(int))
	}
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		locked, err := sqlc.New(tx.Queries()).LockMembers(ctx, c.ID.UUID())
		creators := 0
		for _, m := range locked {
			if m.Role == "creator" {
				creators++
			}
		}
		if err != nil || len(locked) != 3 || creators != 1 {
			t.Errorf("LockMembers = %+v, %v; want 3 rows with one creator", locked, err)
		}
		var pg *pgconn.PgError
		if err := tryLock(); !errors.As(err, &pg) || pg.Code != "55P03" {
			t.Errorf("a second locker got %v, want lock_not_available while the transaction is open", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := tryLock(); err != nil {
		t.Errorf("after the transaction: %v, want the rows unlocked", err)
	}
}

func TestTreasuryWalletQueries_insertThenFindReadsTheWalletOfItsCabal(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	bare := insertBareCabal(t, f, "0000000000")
	_, err := f.q.FindTreasuryWallet(t.Context(), bare)
	wantNoRows(t, "FindTreasuryWallet before the wallet exists", err)
	if err := f.q.InsertTreasuryWallet(t.Context(), sqlc.InsertTreasuryWalletParams{
		CabalID: bare, PrivyWalletID: "wallet-bare", Address: "address-bare", CreatedAt: f.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	got, err := f.q.FindTreasuryWallet(t.Context(), bare)
	if err != nil || got.CabalID != bare || got.PrivyWalletID != "wallet-bare" || got.Address != "address-bare" {
		t.Fatalf("FindTreasuryWallet = %+v, %v", got, err)
	}
	sameInstant(t, "created_at", got.CreatedAt, f.clock.Now())
}

func TestTreasuryWalletQueries_listReturnsEveryCabalsWalletInCabalIDOrder(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	earliestID := insertBareCabal(t, f, "0000000001")
	seeded := []testkit.SeededCabal{testkit.NewCabal(t, f.pool), testkit.NewCabal(t, f.pool)}
	if err := f.q.InsertTreasuryWallet(t.Context(), sqlc.InsertTreasuryWalletParams{
		CabalID: earliestID, PrivyWalletID: "wallet-earliest", Address: "address-earliest", CreatedAt: f.clock.Now(),
	}); err != nil {
		t.Fatal(err)
	}
	all, err := f.q.ListTreasuryWallets(t.Context())
	wantIDs(t, "ListTreasuryWallets", idsOf(all, func(w sqlc.TreasuryWallet) uuid.UUID { return w.CabalID }), err,
		earliestID, seeded[0].ID.UUID(), seeded[1].ID.UUID())
	for _, c := range seeded {
		if !hasWallet(all, c) {
			t.Errorf("wallet of cabal %s is not listed as seeded", c.ID)
		}
	}
}

func insertBareCabal(t *testing.T, f queriesFixture, inviteCode string) uuid.UUID {
	t.Helper()
	params := newCabalParams(testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID(), inviteCode, f.clock.Now())
	n, err := f.q.InsertCabal(t.Context(), params)
	wantRows(t, "InsertCabal", n, 1, err)
	return params.ID
}

func hasWallet(all []sqlc.TreasuryWallet, c testkit.SeededCabal) bool {
	for _, w := range all {
		if w.CabalID == c.ID.UUID() && w.PrivyWalletID == c.PrivyWalletID && w.Address == string(c.TreasuryAddress) {
			return true
		}
	}
	return false
}
