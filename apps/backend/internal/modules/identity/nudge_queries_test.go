package identity_test

import (
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type nudgeRow struct {
	state, status string
	changedAgo    time.Duration
	nudgedAgo     time.Duration
	count         int
}

func seedNudgeRow(t *testing.T, pool *pgxpool.Pool, now time.Time, r nudgeRow) uuid.UUID {
	t.Helper()
	id := testkit.SeedUser(t, pool, testkit.UserOpts{AuthState: r.state, AccountStatus: r.status}).ID.UUID()
	var nudged *time.Time
	if r.nudgedAgo > 0 {
		at := now.Add(-r.nudgedAgo)
		nudged = &at
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE users SET auth_state_changed_at = $2, last_nudged_at = $3, nudge_count = $4 WHERE id = $1`,
		id, now.Add(-r.changedAgo), nudged, r.count); err != nil {
		t.Fatal(err)
	}
	return id
}

func nudgeCutoffs(now time.Time) (changedBefore, nudgedBefore time.Time) {
	return now.Add(-24 * time.Hour), now.Add(-7 * 24 * time.Hour)
}

func TestNudgeCandidates_selectsOnlyUsersDueANudge(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	day := 24 * time.Hour
	due := []uuid.UUID{
		seedNudgeRow(t, pool, now, nudgeRow{state: "AWAITING_PHONE", changedAgo: 25 * time.Hour}),
		seedNudgeRow(t, pool, now, nudgeRow{state: "AWAITING_SOCIALS", changedAgo: day}),
		seedNudgeRow(t, pool, now, nudgeRow{
			state: "AWAITING_PHONE", changedAgo: 9 * day, nudgedAgo: 7 * day,
			count: 1,
		}),
		seedNudgeRow(t, pool, now, nudgeRow{
			state: "AWAITING_SOCIALS", changedAgo: 20 * day, nudgedAgo: 8 * day,
			count: 2,
		}),
	}
	for _, r := range []nudgeRow{
		{state: "AWAITING_PHONE", changedAgo: 23 * time.Hour},
		{state: "AWAITING_PHONE", changedAgo: 9 * day, nudgedAgo: 6 * day, count: 1},
		{state: "AWAITING_PHONE", changedAgo: 30 * day, nudgedAgo: 8 * day, count: 3},
		{state: "AWAITING_PHONE", status: "suspended", changedAgo: 2 * day},
		{state: "AWAITING_PHONE", status: "banned", changedAgo: 2 * day},
		{state: "AWAITING_SOCIALS", status: "deleted", changedAgo: 2 * day},
		{state: "ONBOARDING_COMPLETED", changedAgo: 2 * day},
		{state: "CREATED", changedAgo: 2 * day},
	} {
		seedNudgeRow(t, pool, now, r)
	}
	changed, nudged := nudgeCutoffs(now)
	var got []uuid.UUID
	after := uuid.Nil
	for {
		page, err := sqlc.New(pool).NudgeCandidates(t.Context(), sqlc.NudgeCandidatesParams{
			ChangedBefore: changed, NudgedBefore: nudged, MaxNudges: 3, After: after, Page: 3,
		})
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, page...)
		if len(page) < 3 {
			break
		}
		after = page[len(page)-1]
	}
	slices.SortFunc(due, func(a, b uuid.UUID) int { return slices.Compare(a[:], b[:]) })
	if !slices.Equal(got, due) {
		t.Fatalf("candidates = %v, want %v in id order", got, due)
	}
}

func TestMarkNudged_countsOnlyRowsStillDue(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	phone := seedNudgeRow(t, pool, now, nudgeRow{state: "AWAITING_PHONE", changedAgo: 2 * 24 * time.Hour})
	socials := seedNudgeRow(t, pool, now, nudgeRow{
		state: "AWAITING_SOCIALS", changedAgo: 30 * 24 * time.Hour,
		nudgedAgo: 8 * 24 * time.Hour, count: 2,
	})
	moved := seedNudgeRow(t, pool, now, nudgeRow{state: "ONBOARDING_COMPLETED", changedAgo: 2 * 24 * time.Hour})
	changed, nudged := nudgeCutoffs(now)
	params := sqlc.MarkNudgedParams{
		Now: now, Ids: []uuid.UUID{phone, socials, moved}, ChangedBefore: changed, NudgedBefore: nudged, MaxNudges: 3,
	}
	rows, err := sqlc.New(pool).MarkNudged(t.Context(), params)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(rows, func(a, b sqlc.MarkNudgedRow) int { return int(a.NudgeCount - b.NudgeCount) })
	want := []sqlc.MarkNudgedRow{
		{ID: phone, AuthState: string(domain.AuthAwaitingPhone), NudgeCount: 1},
		{ID: socials, AuthState: string(domain.AuthAwaitingSocials), NudgeCount: 3},
	}
	if !slices.Equal(rows, want) {
		t.Fatalf("marked = %+v, want %+v", rows, want)
	}
	var last time.Time
	if err := pool.QueryRow(t.Context(), `SELECT last_nudged_at FROM users WHERE id = $1`, phone).
		Scan(&last); err != nil || !last.Equal(now) {
		t.Fatalf("last_nudged_at = %s, %v, want %s", last, err, now)
	}
	again, err := sqlc.New(pool).MarkNudged(t.Context(), params)
	if err != nil || len(again) != 0 {
		t.Fatalf("second mark = %+v, %v, want no rows", again, err)
	}
}

func TestUsers_updateAuthStateResetsTheNudgeCount(t *testing.T) {
	t.Parallel()
	f := newUsersFixture(t)
	seeded := testkit.SeedUser(t, f.pool, testkit.UserOpts{AuthState: "ONBOARDING_COMPLETED"})
	if err := f.exec(t, `UPDATE users SET last_nudged_at = $2, nudge_count = 3 WHERE id = $1`,
		seeded.ID.UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	if err := f.updateAuthState(t, seeded.ID, domain.AuthOnboardingCompleted, domain.AuthAwaitingPhone); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.pool.QueryRow(t.Context(), `SELECT nudge_count FROM users WHERE id = $1`, seeded.ID.UUID()).
		Scan(&count); err != nil || count != 0 {
		t.Fatalf("nudge_count = %d, %v, want 0 after a state change", count, err)
	}
}
