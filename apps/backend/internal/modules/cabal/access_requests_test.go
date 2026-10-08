package cabal_test

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const inviteLifetime = 7 * 24 * time.Hour

func fileRequest(t *testing.T, f queriesFixture, cabalID, userID uuid.UUID) (uuid.UUID, int64) {
	t.Helper()
	id := newID()
	n, err := f.q.InsertAccessRequest(t.Context(), sqlc.InsertAccessRequestParams{
		ID: id, CabalID: cabalID, UserID: userID, Direction: "request", Now: f.clock.Now(),
	})
	if err != nil {
		t.Fatalf("InsertAccessRequest: %v", err)
	}
	return id, n
}

func fileInvite(
	t *testing.T,
	f queriesFixture,
	cabalID, userID, inviterID uuid.UUID,
	expiresAt time.Time,
) (uuid.UUID, int64) {
	t.Helper()
	id := newID()
	n, err := f.q.InsertAccessRequest(t.Context(), sqlc.InsertAccessRequestParams{
		ID: id, CabalID: cabalID, UserID: userID, Direction: "invite", Now: f.clock.Now(),
		InvitedBy: pgtype.UUID{Bytes: inviterID, Valid: true},
		ExpiresAt: pgtype.Timestamptz{Time: expiresAt, Valid: true},
	})
	if err != nil {
		t.Fatalf("InsertAccessRequest: %v", err)
	}
	return id, n
}

func decide(t *testing.T, f queriesFixture, cabalID, id, by uuid.UUID, status string) int64 {
	t.Helper()
	n, err := f.q.DecideAccessRequest(t.Context(), sqlc.DecideAccessRequestParams{
		CabalID: cabalID, ID: id, Status: status, DecidedBy: by, Now: f.clock.Now(),
	})
	if err != nil {
		t.Fatalf("DecideAccessRequest: %v", err)
	}
	return n
}

func findRequest(t *testing.T, f queriesFixture, cabalID, id uuid.UUID) sqlc.CabalAccessRequest {
	t.Helper()
	row, err := f.q.FindAccessRequest(t.Context(), sqlc.FindAccessRequestParams{CabalID: cabalID, ID: id})
	if err != nil {
		t.Fatalf("FindAccessRequest: %v", err)
	}
	return row
}

func TestAccessRequestQueries_insertKeepsOnePendingRowPerUserAndCabalInEitherDirection(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	cabalID, creatorID := c.ID.UUID(), c.Creator.ID.UUID()
	userID := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
	first, n := fileRequest(t, f, cabalID, userID)
	wantRows(t, "the first request", n, 1, nil)
	_, n = fileRequest(t, f, cabalID, userID)
	wantRows(t, "a second request", n, 0, nil)
	_, n = fileInvite(t, f, cabalID, userID, creatorID, f.clock.Now().Add(inviteLifetime))
	wantRows(t, "an invite beside the pending request", n, 0, nil)
	pending, err := f.q.FindRequesterAccess(
		t.Context(),
		sqlc.FindRequesterAccessParams{CabalID: cabalID, UserID: userID},
	)
	if err != nil || pending.ID != first || pending.Direction != "request" || pending.Status != "pending" ||
		pending.InvitedBy.Valid || pending.ExpiresAt.Valid || pending.DecidedBy.Valid || pending.DecidedAt.Valid {
		t.Fatalf("pending = %+v, %v; want the first request with no inviter, expiry or decision", pending, err)
	}
	sameInstant(t, "created_at", pending.CreatedAt, f.clock.Now())
	wantRows(t, "denying the request", decide(t, f, cabalID, first, creatorID, "denied"), 1, nil)
	second, n := fileRequest(t, f, cabalID, userID)
	wantRows(t, "a request after a denial", n, 1, nil)
	if again, err := f.q.FindRequesterAccess(t.Context(),
		sqlc.FindRequesterAccessParams{CabalID: cabalID, UserID: userID}); err != nil || again.ID != second {
		t.Fatalf("pending after the denial = %+v, %v; want the new request %s", again, err, second)
	}
}

func TestAccessRequestQueries_anInviteStoresItsInviterAndExpiry(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	userID := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
	expires := f.clock.Now().Add(inviteLifetime)
	id, n := fileInvite(t, f, c.ID.UUID(), userID, c.Creator.ID.UUID(), expires)
	wantRows(t, "the invite", n, 1, nil)
	got := findRequest(t, f, c.ID.UUID(), id)
	if got.Direction != "invite" || got.Status != "pending" || got.UserID != userID ||
		got.InvitedBy != (pgtype.UUID{Bytes: c.Creator.ID.UUID(), Valid: true}) || !got.ExpiresAt.Valid {
		t.Fatalf("invite = %+v, want a pending invite from the creator", got)
	}
	sameInstant(t, "expires_at", got.ExpiresAt.Time, expires)
}

func TestAccessRequestQueries_findReadsOnlyWithinTheRowsOwnCabalAndUser(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	a := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	b := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	userID := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
	id, _ := fileRequest(t, f, a.ID.UUID(), userID)
	if got := findRequest(t, f, a.ID.UUID(), id); got.UserID != userID {
		t.Fatalf("FindAccessRequest = %+v, want the request of %s", got, userID)
	}
	_, err := f.q.FindAccessRequest(t.Context(), sqlc.FindAccessRequestParams{CabalID: b.ID.UUID(), ID: id})
	wantNoRows(t, "FindAccessRequest through another cabal", err)
	_, err = f.q.FindRequesterAccess(
		t.Context(),
		sqlc.FindRequesterAccessParams{CabalID: b.ID.UUID(), UserID: userID},
	)
	wantNoRows(t, "FindRequesterAccess in another cabal", err)
	_, err = f.q.FindRequesterAccess(
		t.Context(),
		sqlc.FindRequesterAccessParams{CabalID: a.ID.UUID(), UserID: newID()},
	)
	wantNoRows(t, "FindRequesterAccess for another user", err)
}

func TestAccessRequestQueries_decideMovesAPendingRowExactlyOnceInItsOwnCabal(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	a := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"), testkit.WithMembers(2))
	b := testkit.NewCabal(t, f.pool)
	cabalID, creatorID := a.ID.UUID(), a.Creator.ID.UUID()
	id, _ := fileRequest(t, f, cabalID, testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID())
	wantRows(
		t,
		"deciding through another cabal",
		decide(t, f, b.ID.UUID(), id, b.Creator.ID.UUID(), "approved"),
		0,
		nil,
	)
	if got := findRequest(t, f, cabalID, id); got.Status != "pending" {
		t.Fatalf("status after a wrong-cabal decision = %s, want pending", got.Status)
	}
	f.clock.Advance(time.Minute)
	decidedAt := f.clock.Now()
	wantRows(t, "the decision", decide(t, f, cabalID, id, creatorID, "approved"), 1, nil)
	f.clock.Advance(time.Minute)
	wantRows(t, "a racing second decision", decide(t, f, cabalID, id, a.Members[1].ID.UUID(), "denied"), 0, nil)
	got := findRequest(t, f, cabalID, id)
	if got.Status != "approved" || got.DecidedBy != (pgtype.UUID{Bytes: creatorID, Valid: true}) ||
		!got.DecidedAt.Valid {
		t.Fatalf("request = %+v, want approved by the creator and untouched by the second decision", got)
	}
	sameInstant(t, "decided_at", got.DecidedAt.Time, decidedAt)
}

func TestAccessRequestQueries_expireMovesOnlyAPendingInviteAndRecordsNoDecider(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request"))
	cabalID, creatorID := c.ID.UUID(), c.Creator.ID.UUID()
	seed := func() uuid.UUID { return testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID() }
	expire := func(id uuid.UUID) int64 {
		t.Helper()
		n, err := f.q.ExpireAccessRequest(t.Context(), sqlc.ExpireAccessRequestParams{
			CabalID: cabalID, ID: id, Now: f.clock.Now(),
		})
		if err != nil {
			t.Fatal(err)
		}
		return n
	}
	pendingInvite, _ := fileInvite(t, f, cabalID, seed(), creatorID, f.clock.Now())
	acceptedInvite, _ := fileInvite(t, f, cabalID, seed(), creatorID, f.clock.Now())
	request, _ := fileRequest(t, f, cabalID, seed())
	decide(t, f, cabalID, acceptedInvite, creatorID, "approved")
	f.clock.Advance(time.Hour)
	wantRows(t, "expiring a pending invite", expire(pendingInvite), 1, nil)
	wantRows(t, "expiring it again", expire(pendingInvite), 0, nil)
	wantRows(t, "expiring an accepted invite", expire(acceptedInvite), 0, nil)
	wantRows(t, "expiring a request", expire(request), 0, nil)
	got := findRequest(t, f, cabalID, pendingInvite)
	if got.Status != "expired" || got.DecidedBy.Valid || !got.DecidedAt.Valid {
		t.Fatalf("expired invite = %+v, want expired with no decider", got)
	}
	sameInstant(t, "decided_at", got.DecidedAt.Time, f.clock.Now())
	if got := findRequest(t, f, cabalID, acceptedInvite).Status; got != "approved" {
		t.Errorf("accepted invite status = %s, want approved", got)
	}
	if got := findRequest(t, f, cabalID, request).Status; got != "pending" {
		t.Errorf("request status = %s, want pending: requests do not expire", got)
	}
}

func TestAccessRequestQueries_listDueInvitesReturnsPendingInvitesPastTheirExpiryOldestFirst(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	cabalID, creatorID := c.ID.UUID(), c.Creator.ID.UUID()
	now := f.clock.Now()
	invite := func(expires time.Time) uuid.UUID {
		id, _ := fileInvite(
			t,
			f,
			cabalID,
			testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID(),
			creatorID,
			expires,
		)
		return id
	}
	newer, older := invite(now.Add(-time.Hour)), invite(now.Add(-2*time.Hour))
	invite(now)
	invite(now.Add(time.Hour))
	decide(t, f, cabalID, invite(now.Add(-3*time.Hour)), creatorID, "denied")
	fileRequest(t, f, cabalID, testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID())
	due, err := f.q.ListDueInvites(t.Context(), sqlc.ListDueInvitesParams{Now: now, MaxRows: 10})
	if err != nil || len(due) != 2 || due[0].ID != older || due[1].ID != newer {
		t.Fatalf("ListDueInvites = %+v, %v; want the two pending invites past expiry, oldest first", due, err)
	}
	if due[0].CabalID != cabalID || due[0].InvitedBy != (pgtype.UUID{Bytes: creatorID, Valid: true}) {
		t.Errorf("due invite = %+v, want the cabal and inviter carried", due[0])
	}
	sameInstant(t, "expires_at", due[0].ExpiresAt.Time, now.Add(-2*time.Hour))
	if capped, err := f.q.ListDueInvites(t.Context(), sqlc.ListDueInvitesParams{Now: now, MaxRows: 1}); err != nil ||
		len(capped) != 1 || capped[0].ID != older {
		t.Fatalf("ListDueInvites with a limit of 1 = %+v, %v; want only the oldest", capped, err)
	}
}

type pendingMix struct {
	a, b                             testkit.SeededCabal
	users                            []uuid.UUID
	r1, r2, i1, i2, elsewhere, stale uuid.UUID
}

func seedPendingMix(t *testing.T, f queriesFixture) pendingMix {
	t.Helper()
	m := pendingMix{
		a: testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request")),
		b: testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request")),
	}
	for range 8 {
		m.users = append(m.users, testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID())
	}
	aID, bID, creatorID := m.a.ID.UUID(), m.b.ID.UUID(), m.a.Creator.ID.UUID()
	tick := func() time.Time { f.clock.Advance(time.Second); return f.clock.Now() }
	tick()
	m.r1, _ = fileRequest(t, f, aID, m.users[0])
	m.i1, _ = fileInvite(t, f, aID, m.users[3], creatorID, tick().Add(inviteLifetime))
	m.r2, _ = fileRequest(t, f, aID, m.users[1])
	denied, _ := fileRequest(t, f, aID, m.users[2])
	decide(t, f, aID, denied, creatorID, "denied")
	m.i2, _ = fileInvite(t, f, aID, m.users[4], creatorID, tick().Add(inviteLifetime))
	m.stale, _ = fileInvite(t, f, aID, m.users[6], creatorID, tick())
	accepted, _ := fileInvite(t, f, aID, m.users[7], creatorID, tick().Add(inviteLifetime))
	decide(t, f, aID, accepted, m.users[7], "approved")
	expired, _ := fileInvite(t, f, aID, m.users[5], creatorID, tick())
	if n, err := f.q.ExpireAccessRequest(t.Context(),
		sqlc.ExpireAccessRequestParams{CabalID: aID, ID: expired, Now: tick()}); err != nil || n != 1 {
		t.Fatalf("ExpireAccessRequest = %d, %v", n, err)
	}
	declined, _ := fileInvite(t, f, bID, m.users[3], m.b.Creator.ID.UUID(), tick().Add(inviteLifetime))
	decide(t, f, bID, declined, m.users[3], "denied")
	m.elsewhere, _ = fileInvite(t, f, bID, m.users[3], m.b.Creator.ID.UUID(), tick().Add(inviteLifetime))
	fileRequest(t, f, bID, m.users[0])
	return m
}

func TestAccessRequestQueries_pendingRequestsOfACabalExcludeInvitesDecisionsAndOtherCabals(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	m := seedPendingMix(t, f)
	got, err := f.q.ListPendingRequestsForCabal(t.Context(), m.a.ID.UUID())
	wantIDs(
		t,
		"ListPendingRequestsForCabal",
		idsOf(got, func(r sqlc.ListPendingRequestsForCabalRow) uuid.UUID { return r.ID }),
		err,
		m.r1,
		m.r2,
	)
}

func TestAccessRequestQueries_pendingInvitesOfACabalExcludeRequestsDecisionsOtherCabalsAndStaleInvites(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	m := seedPendingMix(t, f)
	got, err := f.q.ListPendingInvitesForCabal(t.Context(),
		sqlc.ListPendingInvitesForCabalParams{CabalID: m.a.ID.UUID(), Now: f.clock.Now()})
	wantIDs(t, "ListPendingInvitesForCabal",
		idsOf(got, func(r sqlc.ListPendingInvitesForCabalRow) uuid.UUID { return r.ID }), err, m.i1, m.i2)
	if len(got) == 2 && (got[0].UserID != m.users[3] || got[1].UserID != m.users[4]) {
		t.Errorf("invitees = %s and %s, want users 3 and 4", got[0].UserID, got[1].UserID)
	}
}

func TestAccessRequestQueries_pendingInvitesOfAUserSpanCabalsInTheOrderTheyCameAndSkipStaleOnes(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	m := seedPendingMix(t, f)
	list := func(user uuid.UUID) ([]uuid.UUID, error) {
		rows, err := f.q.ListPendingInvitesForUser(t.Context(),
			sqlc.ListPendingInvitesForUserParams{UserID: user, Now: f.clock.Now()})
		return idsOf(rows, func(r sqlc.ListPendingInvitesForUserRow) uuid.UUID { return r.ID }), err
	}
	got, err := list(m.users[3])
	wantIDs(t, "ListPendingInvitesForUser", got, err, m.i1, m.elsewhere)
	rows, err := f.q.ListPendingInvitesForUser(t.Context(),
		sqlc.ListPendingInvitesForUserParams{UserID: m.users[3], Now: f.clock.Now()})
	if err != nil || len(rows) != 2 || rows[0].CabalID != m.a.ID.UUID() || rows[1].CabalID != m.b.ID.UUID() {
		t.Errorf("cabals of the invites = %+v, %v; want a then b", rows, err)
	}
	none, err := list(m.users[0])
	wantIDs(t, "ListPendingInvitesForUser for someone who only asked to join", none, err)
	stale, err := list(m.users[6])
	wantIDs(t, "ListPendingInvitesForUser for a user whose invite passed its expiry", stale, err)
}

func TestAccessRequestQueries_anInviteStaysListedThroughItsExpiryInstantAndNotAfter(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	c := testkit.NewCabal(t, f.pool)
	user := testkit.SeedUser(t, f.pool, testkit.UserOpts{}).ID.UUID()
	expires := f.clock.Now().Add(inviteLifetime)
	id, _ := fileInvite(t, f, c.ID.UUID(), user, c.Creator.ID.UUID(), expires)
	for _, tt := range []struct {
		now  time.Time
		want []uuid.UUID
	}{
		{expires.Add(-time.Microsecond), []uuid.UUID{id}},
		{expires, []uuid.UUID{id}},
		{expires.Add(time.Microsecond), nil},
	} {
		byCabal, err := f.q.ListPendingInvitesForCabal(t.Context(),
			sqlc.ListPendingInvitesForCabalParams{CabalID: c.ID.UUID(), Now: tt.now})
		wantIDs(t, "cabal list at "+tt.now.Sub(expires).String(),
			idsOf(byCabal, func(r sqlc.ListPendingInvitesForCabalRow) uuid.UUID { return r.ID }), err, tt.want...)
		byUser, err := f.q.ListPendingInvitesForUser(t.Context(),
			sqlc.ListPendingInvitesForUserParams{UserID: user, Now: tt.now})
		wantIDs(t, "user list at "+tt.now.Sub(expires).String(),
			idsOf(byUser, func(r sqlc.ListPendingInvitesForUserRow) uuid.UUID { return r.ID }), err, tt.want...)
	}
}

func accessRequestQuery(t *testing.T, name string) string {
	t.Helper()
	body, err := os.ReadFile("../../../queries/cabal/access_requests.sql")
	if err != nil {
		t.Fatal(err)
	}
	_, rest, ok := strings.Cut(string(body), "-- name: "+name+" ")
	if !ok {
		t.Fatalf("access_requests.sql has no query %s", name)
	}
	_, query, _ := strings.Cut(rest, "\n")
	query, _, _ = strings.Cut(query, ";")
	n := 0
	return regexp.MustCompile(`sqlc\.n?arg\(\w+\)`).ReplaceAllStringFunc(query, func(string) string {
		n++
		return "$" + strconv.Itoa(n)
	})
}

func TestAccessRequestQueries_theInviteInboxReadsThePendingUserIndex(t *testing.T) {
	t.Parallel()
	f := newQueries(t)
	query := accessRequestQuery(t, "ListPendingInvitesForUser")
	cabals := []testkit.SeededCabal{
		testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request")),
		testkit.NewCabal(t, f.pool, testkit.WithJoinMode("request")),
	}
	for _, statement := range []string{
		`INSERT INTO users (id, privy_user_id, login_provider, auth_state, auth_state_changed_at, account_status,
			created_at, updated_at)
		SELECT gen_random_uuid(), 'did:privy:inbox-' || g, 'sms', 'CREATED', now(), 'active', now(), now()
		FROM generate_series(1, 4000) g`,
		`INSERT INTO cabal_access_requests (id, cabal_id, user_id, direction, invited_by, expires_at, created_at)
		SELECT gen_random_uuid(), c.id, u.id, 'invite', c.creator_id, now() + interval '7 days', now()
		FROM users u, cabals c WHERE u.privy_user_id LIKE 'did:privy:inbox-%' AND c.id = ANY($1::uuid[])`,
		`ANALYZE cabal_access_requests`,
	} {
		args := []any{}
		if strings.Contains(statement, "$1") {
			args = append(args, []uuid.UUID{cabals[0].ID.UUID(), cabals[1].ID.UUID()})
		}
		if _, err := f.pool.Exec(t.Context(), statement, args...); err != nil {
			t.Fatal(err)
		}
	}
	plan := f.indexOnlyPlan(t, query, newID(), f.clock.Now())
	if !strings.Contains(plan, "cabal_access_requests_pending_user_idx") {
		t.Fatalf("plan does not use cabal_access_requests_pending_user_idx:\n%s", plan)
	}
}

func (f queriesFixture) indexOnlyPlan(t *testing.T, query string, args ...any) string {
	t.Helper()
	var plan strings.Builder
	err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		if _, err := tx.Queries().Exec(ctx, `SET LOCAL enable_seqscan = off`); err != nil {
			return err
		}
		rows, err := tx.Queries().Query(ctx, "EXPLAIN "+query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				return err
			}
			plan.WriteString(line + "\n")
		}
		return rows.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return plan.String()
}
