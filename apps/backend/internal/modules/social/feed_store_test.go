package social_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/feedtest"
)

type feedFixture struct {
	pool  *pgxpool.Pool
	gen   *testkit.IDs
	clock *testkit.Clock
}

func newFeedFixture(t *testing.T) feedFixture {
	t.Helper()
	return feedFixture{
		pool:  testkit.DB(t),
		gen:   testkit.NewIDs(590),
		clock: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond)),
	}
}

func (f feedFixture) item(t *testing.T, edits ...func(*feed.Item)) uuid.UUID {
	t.Helper()
	return feedtest.Item(t, f.pool, f.clock, f.gen, edits...)
}

func wantSQLState(t *testing.T, err error, state string) {
	t.Helper()
	var pg *pgconn.PgError
	if !errors.As(err, &pg) || pg.Code != state {
		t.Fatalf("err = %v, want SQLSTATE %s", err, state)
	}
}

func TestFeedObjects_refuseASecondRowForTheSameSourceAndKind(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ref := f.gen.NewV7()
	insert := `INSERT INTO feed_objects (id, kind, ref_type, ref_id, title, payload, created_at, updated_at)
		VALUES ($1, 'trade', 'swaps', $2, 't', '{}', now(), now())`
	if _, err := f.pool.Exec(t.Context(), insert, f.gen.NewV7(), ref); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(t.Context(), insert, f.gen.NewV7(), ref)
	wantSQLState(t, err, "23505")
}

func TestFeedComments_refuseAReplyWhoseParentIsOnAnotherItem(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	first, second := f.item(t), f.item(t)
	insert := `INSERT INTO feed_comments (id, feed_object_id, author_id, parent_comment_id, body, created_at)
		VALUES ($1, $2, $3, $4, 'hi', now())`
	parent := f.gen.NewV7()
	if _, err := f.pool.Exec(t.Context(), insert, parent, first, f.gen.NewV7(), nil); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), insert, f.gen.NewV7(), first, f.gen.NewV7(), parent); err != nil {
		t.Fatalf("reply on the same item: %v", err)
	}
	_, err := f.pool.Exec(t.Context(), insert, f.gen.NewV7(), second, f.gen.NewV7(), parent)
	wantSQLState(t, err, "23503")
}

func TestFeedStoreUpsertItem_refreshesTheSnapshotAndKeepsIdentityAndStatus(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ref, cabal := f.gen.NewV7(), ids.CabalIDFrom(f.gen.NewV7())
	proposal := func(name string) func(*feed.Item) {
		return func(it *feed.Item) {
			it.Kind, it.RefID, it.CabalID, it.Status = feed.KindProposal, ref, cabal, "open"
			it.Payload.CabalName = name
		}
	}
	first := f.item(t, proposal("Alpha"))
	created := f.clock.Now()
	f.clock.Advance(time.Minute)
	againWithNewStatus := func(it *feed.Item) { proposal("Bravo")(it); it.Status = "passed" }
	if again := f.item(t, againWithNewStatus); again != first {
		t.Fatalf("second upsert id = %s, want the first row's %s", again, first)
	}
	var (
		title, name, status string
		createdAt, updated  time.Time
		count               int
	)
	err := f.pool.QueryRow(t.Context(), `SELECT title, cabal_name, status, created_at, updated_at,
		(SELECT count(*) FROM feed_objects) FROM feed_objects WHERE id = $1`, first).
		Scan(&title, &name, &status, &createdAt, &updated, &count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || title != "Bravo proposed buying $500 of AAPLx" || name != "Bravo" || status != "open" ||
		!createdAt.Equal(created) || !updated.Equal(f.clock.Now()) {
		t.Fatalf("row = %q %q %q created %s updated %s (%d rows)", title, name, status, createdAt, updated, count)
	}
}

func TestFeedStoreUpsertItem_refusesAKindWithNoSourceTable(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	_, err := adapters.NewFeedStore(f.pool).UpsertItem(t.Context(), f.gen.NewV7(),
		feed.Item{Kind: "news", RefID: f.gen.NewV7()}, f.clock.Now())
	wantCode(t, err, errs.CodeInternal)
}

func TestFeedStoreUpdateStatus_movesOnlyFromTheExpectedStatus(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ref := f.gen.NewV7()
	id := f.item(t, func(it *feed.Item) { it.Kind, it.RefID, it.Status = feed.KindProposal, ref, "open" })
	store := adapters.NewFeedStore(f.pool)
	change := feed.StatusChange{
		Kind: feed.KindProposal, RefID: ref, From: "open", To: "passed",
		Payload: feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", VoterCount: 3, YesVotes: 2},
	}
	f.clock.Advance(time.Minute)
	moved, err := store.UpdateStatus(t.Context(), change, f.clock.Now())
	if err != nil || !moved {
		t.Fatalf("first move = %v, %v, want true", moved, err)
	}
	change.To = "expired"
	if moved, err := store.UpdateStatus(t.Context(), change, f.clock.Now()); err != nil || moved {
		t.Fatalf("stale move = %v, %v, want false", moved, err)
	}
	var status, title string
	var updated time.Time
	err = f.pool.QueryRow(t.Context(), `SELECT status, title, updated_at FROM feed_objects WHERE id = $1`, id).
		Scan(&status, &title, &updated)
	if err != nil {
		t.Fatal(err)
	}
	if status != "passed" || title != "Alpha proposed buying AAPLx" || !updated.Equal(f.clock.Now()) {
		t.Fatalf("row = %q %q %s", status, title, updated)
	}
}

func TestFeedStoreUpdateStatus_reportsADatabaseFailure(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := adapters.NewFeedStore(f.pool).UpdateStatus(ctx, feed.StatusChange{Kind: feed.KindProposal}, f.clock.Now())
	if err == nil {
		t.Fatal("UpdateStatus on a cancelled context succeeded")
	}
}
