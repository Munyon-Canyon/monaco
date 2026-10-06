package social_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const (
	createdHandler = "social.feed"
	profileHandler = "social.feed.profile_updated"
	cabalHandler   = "social.feed.cabal_updated"
	renameRows     = 1200
	hintKey        = "global.feed"
	hintSubject    = "hint." + hintKey
)

type renamer struct {
	pool  *pgxpool.Pool
	gen   *testkit.IDs
	clock *testkit.Clock
	uow   *db.UnitOfWork
	users *fakes.Identity
	names *cabalNames
	conn  *bus.Conn
	alice ids.UserID
	bob   ids.UserID
}

func newRenamer(t *testing.T) renamer {
	t.Helper()
	g := testkit.NewIDs(668)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Second))
	pool := testkit.DB(t)
	return renamer{
		pool: pool, gen: g, clock: clk, uow: db.New(pool, g, clk),
		users: fakes.NewIdentity(nil, nil), names: &cabalNames{current: map[ids.CabalID]string{}},
		alice: ids.NewUserID(g), bob: ids.NewUserID(g),
	}
}

func (r renamer) knowing(cards ...identity.UserCard) renamer {
	r.users = fakes.NewIdentity(cards, nil)
	return r
}

func (r renamer) event(t *testing.T, ev events.Event) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	ctx := observability.WithActor(t.Context(), "system:feed-rename-test")
	err := r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		if err := tx.Events.Append(ctx, ev); err != nil {
			return err
		}
		return tx.Queries().QueryRow(ctx, `SELECT id FROM events ORDER BY id DESC LIMIT 1`).Scan(&id)
	})
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func (r renamer) deliver(t *testing.T, handler string, ev events.Event) (bool, error) {
	t.Helper()
	return r.deliverAs(t.Context(), t, r.event(t, ev), handler, ev)
}

func (r renamer) deliverAs(
	ctx context.Context, t *testing.T, id uuid.UUID, handler string, ev events.Event,
) (bool, error) {
	t.Helper()
	deps := module.Deps{Pool: r.pool, UoW: r.uow, IDs: r.gen, Clock: r.clock, Bus: r.conn}
	ctx = observability.WithEventID(ctx, ids.EventIDFrom(id))
	for _, c := range social.New(deps, social.WithUsers(r.users), social.WithCabals(r.names)).Consumers() {
		for _, h := range c.Handlers {
			if h.Name == handler {
				return bus.Deliver(ctx, r.uow, r.clock, h, ids.EventIDFrom(id), ev)
			}
		}
	}
	t.Fatalf("handler %s is not registered", handler)
	return false, nil
}

func (r renamer) mustDeliver(t *testing.T, handler string, ev events.Event) {
	t.Helper()
	if duplicate, err := r.deliver(t, handler, ev); err != nil || duplicate {
		t.Fatalf("%s = duplicate %t, %v; want a first delivery that succeeds", handler, duplicate, err)
	}
}

func (r renamer) seed(t *testing.T, n int, item feed.Item) []uuid.UUID {
	t.Helper()
	rows, refs := make([]uuid.UUID, n), make([]uuid.UUID, n)
	for i := range n {
		rows[i], refs[i] = r.gen.NewV7(), r.gen.NewV7()
	}
	kind, refType := string(item.Kind), string(item.Kind.RefType())
	cabal, actor := item.CabalID.UUID(), item.ActorID.UUID()
	title, payload := feed.RenderTitle(item.Kind, item.Payload), item.Payload.JSON()
	r.exec(t, `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, title, payload, created_at, updated_at)
		SELECT id, $3, $4, ref_id, nullif($5::uuid, $6::uuid), nullif($7::text, ''), nullif($8::uuid, $6::uuid),
			$9, $10, $11, $11
		FROM unnest($1::uuid[], $2::uuid[]) AS seeded(id, ref_id)`,
		rows, refs, kind, refType, cabal, uuid.Nil, item.Payload.CabalName, actor, title, payload, r.clock.Now())
	return rows
}

func joined(actor ids.UserID, cabal ids.CabalID, name string) feed.Item {
	return feed.Item{
		Kind: feed.KindMemberJoined, ActorID: actor, CabalID: cabal,
		Payload: feed.Payload{CabalName: "Alpha", ActorName: name},
	}
}

func withActor(item feed.Item, actor ids.UserID) feed.Item {
	item.ActorID = actor
	return item
}

func withActorName(item feed.Item, name string) feed.Item {
	item.Payload.ActorName = name
	return item
}

func withChange(item feed.Item, bps int64) feed.Item {
	item.Payload.ChangeBps = bps
	return item
}

func buy(kind feed.Kind, cabal ids.CabalID) feed.Item {
	payload := feed.Payload{CabalName: "Alpha", Symbol: "AAPLx", Action: feed.ActionBuy}
	payload.USDCMicros = money.MicrosFromUint64(500_000_000)
	return feed.Item{Kind: kind, CabalID: cabal, Payload: payload}
}

func (r renamer) count(t *testing.T, where string, args ...any) int {
	t.Helper()
	var n int
	if err := r.pool.QueryRow(t.Context(), "SELECT count(*) FROM feed_objects WHERE "+where, args...).
		Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r renamer) versions(t *testing.T) string {
	t.Helper()
	var got string
	err := r.pool.QueryRow(t.Context(), `SELECT
		coalesce((SELECT string_agg(id::text || ':' || xmin::text, ',' ORDER BY id) FROM feed_objects), '') ||
		coalesce((SELECT string_agg(cabal_id::text || ':' || xmin::text, ',' ORDER BY cabal_id) FROM feed_cabals), '')`,
	).Scan(&got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func (r renamer) found(t *testing.T, q string) int {
	t.Helper()
	n := 0
	query := app.FeedQuery{Filter: app.FeedFilter{Q: q}, Limit: app.FeedPageMax}
	for {
		page, err := app.ListFeed(t.Context(), r.pool, query)
		if err != nil {
			t.Fatal(err)
		}
		n += len(page.Items)
		if page.Next == nil {
			return n
		}
		query.After = page.Next
	}
}

func (r renamer) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (r renamer) cabal(t *testing.T, name string) ids.CabalID {
	t.Helper()
	id := ids.CabalIDFrom(r.gen.NewV7())
	r.exec(
		t,
		`INSERT INTO feed_cabals (cabal_id, name, updated_at) VALUES ($1, $2, $3)`,
		id.UUID(),
		name,
		r.clock.Now(),
	)
	return id
}

func profileUpdated(user uuid.UUID, name string, fields ...string) events.UserProfileUpdated {
	return events.UserProfileUpdated{V: 1, UserID: user, Fields: fields, DisplayName: name}
}

type cabalNames struct {
	current map[ids.CabalID]string
	err     error
}

func (c *cabalNames) Cabal(_ context.Context, id ids.CabalID) (cabalport.CabalView, error) {
	return cabalport.CabalView{ID: id, Name: c.current[id]}, c.err
}

func (r renamer) renamed(cabal ids.CabalID, actor ids.UserID, name string) events.CabalUpdated {
	r.names.current[cabal] = name
	return cabalRenamed(cabal.UUID(), actor.UUID(), name)
}

func cabalRenamed(cabal, actor uuid.UUID, name string) events.CabalUpdated {
	return events.CabalUpdated{V: 1, CabalID: cabal, ActorID: actor, Changes: events.CabalChanges{Name: &name}}
}

func wantHints(t *testing.T, conn *bus.Conn, sub *testkit.CoreSubscription, n int) {
	t.Helper()
	for range n {
		sub.Next(t)
	}
	conn.PublishHint(t.Context(), hintKey, []byte("end"))
	if got := string(sub.Next(t)); got != "end" {
		t.Fatalf("an extra hint arrived before the marker; want exactly %d", n)
	}
}

func TestFeedRename_Profile(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	r := newRenamer(t)
	r.conn = b.Conn
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice", DisplayName: "Quillen"})
	cabal := ids.CabalIDFrom(r.gen.NewV7())
	r.seed(t, renameRows, joined(r.alice, cabal, "alice"))
	r.seed(t, 3, joined(r.bob, cabal, "bob"))
	proposal := r.seed(t, 1, withActorName(withActor(buy(feed.KindProposal, cabal), r.alice), "alice"))[0]
	sub := testkit.SubscribeCore(t, b, hintSubject)
	seededAt := r.clock.Now()
	r.clock.Advance(time.Hour)
	ev := profileUpdated(r.alice.UUID(), "Quillen", "display_name")
	id := r.event(t, ev)

	if duplicate, err := r.deliverAs(t.Context(), t, id, profileHandler, ev); err != nil || duplicate {
		t.Fatalf("first delivery = duplicate %t, %v; want it to succeed", duplicate, err)
	}

	renamed := r.count(t, `actor_id = $1 AND kind = 'member_joined' AND title = 'Quillen joined Alpha'
		AND payload->>'actor_name' = 'Quillen' AND cabal_name = 'Alpha' AND updated_at = $2`,
		r.alice.UUID(), r.clock.Now())
	if renamed != renameRows {
		t.Fatalf("renamed rows = %d, want %d", renamed, renameRows)
	}
	untouched := r.count(t, `updated_at = $1 AND actor_id = $2 AND title = 'bob joined Alpha'`, seededAt, r.bob.UUID())
	if untouched != 3 {
		t.Fatalf("untouched bystander rows = %d, want bob's 3", untouched)
	}
	proposed := r.count(t, `id = $1 AND title = 'Quillen proposed buying $500.00 of AAPLx in Alpha'
		AND payload->>'actor_name' = 'Quillen' AND updated_at = $2`, proposal, r.clock.Now())
	if proposed != 1 {
		t.Fatalf("renamed proposal rows = %d, want alice's proposal", proposed)
	}
	if got := r.found(t, "Quillen"); got != renameRows+1 {
		t.Fatalf("q=Quillen finds %d rows, want %d", got, renameRows+1)
	}
	if got := r.found(t, "alice"); got != 0 {
		t.Fatalf("q=alice still finds %d rows", got)
	}
	wantHints(t, b.Conn, sub, 1)

	again := r.versions(t)
	if duplicate, err := r.deliverAs(t.Context(), t, id, profileHandler, ev); err != nil || !duplicate {
		t.Fatalf("redelivery = duplicate %t, %v; want a duplicate", duplicate, err)
	}
	r.mustDeliver(t, profileHandler, ev)
	if r.versions(t) != again {
		t.Fatal("a second event with the same name rewrote rows")
	}
	wantHints(t, b.Conn, sub, 0)
}

func TestFeedRename_Cabal(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	r := newRenamer(t)
	r.conn = b.Conn
	r = r.knowing(
		identity.UserCard{ID: r.alice, Handle: "alice"}, identity.UserCard{ID: r.bob, Handle: "bob"},
	)
	cabal, other := ids.CabalIDFrom(r.gen.NewV7()), ids.CabalIDFrom(r.gen.NewV7())
	r.mustDeliver(t, createdHandler, events.CabalCreated{
		V: 1, CabalID: cabal.UUID(), CreatorID: r.alice.UUID(), Name: "Alpha",
	})
	r.seed(t, renameRows/3, buy(feed.KindTrade, cabal))
	r.seed(t, renameRows/3, buy(feed.KindProposal, cabal))
	r.seed(t, renameRows/3, joined(r.bob, cabal, "bob"))
	r.seed(t, 5, feed.Item{
		Kind: feed.KindTrade, CabalID: other, Payload: feed.Payload{CabalName: "Bravo", Symbol: "TSLAx"},
	})
	r.seed(t, 1, feed.Item{Kind: feed.KindPriceMove, Payload: feed.Payload{Symbol: "AAPLx", ChangeBps: 1000}})
	sub := testkit.SubscribeCore(t, b, hintSubject)
	r.clock.Advance(time.Hour)

	r.mustDeliver(t, cabalHandler, r.renamed(cabal, r.alice, "Quorum"))

	wantTitles := map[string]int{
		"Quorum bought $500 of AAPLx":                         renameRows / 3,
		"A member proposed buying $500.00 of AAPLx in Quorum": renameRows / 3,
		"bob joined Quorum":                                   renameRows / 3,
		"alice started Quorum":                                1,
	}
	for title, want := range wantTitles {
		got := r.count(t, `cabal_id = $1 AND title = $2 AND cabal_name = 'Quorum' AND payload->>'cabal_name' = 'Quorum'
			AND updated_at = $3`, cabal.UUID(), title, r.clock.Now())
		if got != want {
			t.Fatalf("rows titled %q = %d, want %d", title, got, want)
		}
	}
	var name string
	if err := r.pool.QueryRow(t.Context(), `SELECT name FROM feed_cabals WHERE cabal_id = $1`, cabal.UUID()).
		Scan(&name); err != nil ||
		name != "Quorum" {
		t.Fatalf("feed_cabals name = %q, %v; want Quorum", name, err)
	}
	if got := r.found(t, "Quorum"); got != renameRows+1 {
		t.Fatalf("q=Quorum finds %d rows, want %d", got, renameRows+1)
	}
	if got := r.found(t, "Alpha"); got != 0 {
		t.Fatalf("q=Alpha still finds %d rows", got)
	}
	if got := r.found(t, "Bravo"); got != 5 {
		t.Fatalf("q=Bravo finds %d rows, want the other cabal's 5", got)
	}
	wantHints(t, b.Conn, sub, 1)
}

func TestFeedRename_ProfileWritesNothingWhenTheNameStays(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		card    identity.UserCard
		stored  string
		fields  []string
		failing bool
	}{
		"a photo only, with no identity read": {stored: "alice", fields: []string{"photo"}, failing: true},
		"a handle behind a display name": {
			card: identity.UserCard{Handle: "second", DisplayName: "Quillen"}, stored: "Quillen", fields: []string{"handle"},
		},
		"the name the rows already show": {
			card: identity.UserCard{Handle: "alice", DisplayName: "Quillen"}, stored: "Quillen", fields: []string{"display_name"},
		},
		"a user identity does not know": {stored: "alice", fields: []string{"display_name"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			b := testkit.NATS(t)
			r := newRenamer(t)
			r.conn = b.Conn
			if tc.card.Handle != "" {
				tc.card.ID = r.alice
				r = r.knowing(tc.card)
			}
			if tc.failing {
				r.users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
			}
			r.seed(t, 20, joined(r.alice, ids.CabalIDFrom(r.gen.NewV7()), tc.stored))
			sub := testkit.SubscribeCore(t, b, hintSubject)
			before := r.versions(t)
			r.clock.Advance(time.Hour)

			r.mustDeliver(t, profileHandler, profileUpdated(r.alice.UUID(), "", tc.fields...))

			if r.versions(t) != before {
				t.Fatal("rows were rewritten")
			}
			wantHints(t, b.Conn, sub, 0)
		})
	}
}

func TestFeedRename_ProfileFollowsTheHandleWithoutADisplayName(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "wren"})
	r.seed(t, 20, joined(r.alice, ids.CabalIDFrom(r.gen.NewV7()), "alice"))

	r.mustDeliver(t, profileHandler, profileUpdated(r.alice.UUID(), "", "handle"))

	if got := r.found(t, "wren"); got != 20 {
		t.Fatalf("q=wren finds %d rows, want 20", got)
	}
}

func TestFeedRename_CabalWritesNothingWithoutANameChange(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	r := newRenamer(t)
	r.conn = b.Conn
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice"})
	cabal := ids.CabalIDFrom(r.gen.NewV7())
	r.mustDeliver(t, createdHandler, events.CabalCreated{
		V: 1, CabalID: cabal.UUID(), CreatorID: r.alice.UUID(), Name: "Alpha",
	})
	r.seed(t, 20, joined(r.bob, cabal, "bob"))
	sub := testkit.SubscribeCore(t, b, hintSubject)
	before := r.versions(t)
	picture := "https://cdn.example.com/cabals/alpha.png"
	update := events.CabalUpdated{
		V: 1, CabalID: cabal.UUID(), ActorID: r.alice.UUID(), Changes: events.CabalChanges{PictureURL: &picture},
	}

	r.mustDeliver(t, cabalHandler, update)
	update.CabalID = r.gen.NewV7()
	r.mustDeliver(t, cabalHandler, update)

	if r.versions(t) != before {
		t.Fatal("rows were rewritten")
	}
	wantHints(t, b.Conn, sub, 0)
}

func TestFeedRename_CabalWaitsForItsCreation(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice"})
	cabal := ids.CabalIDFrom(r.gen.NewV7())
	created := events.CabalCreated{V: 1, CabalID: cabal.UUID(), CreatorID: r.alice.UUID(), Name: "Alpha"}
	rename := r.renamed(cabal, r.alice, "Quorum")
	id := r.event(t, rename)

	_, err := r.deliverAs(t.Context(), t, id, cabalHandler, rename)
	wantCode(t, err, errs.CodeFeedItemPending)
	r.mustDeliver(t, createdHandler, created)
	if duplicate, err := r.deliverAs(t.Context(), t, id, cabalHandler, rename); err != nil || duplicate {
		t.Fatalf("redelivery after the cabal exists = duplicate %t, %v; want it to succeed", duplicate, err)
	}

	if got := r.found(t, "Quorum"); got != 1 {
		t.Fatalf("q=Quorum finds %d rows after the retry, want the created item", got)
	}
}

func TestFeedRename_returnsEachFailure(t *testing.T) {
	t.Parallel()
	const badPayload = `{"actor_name": "alice", "cabal_name": "Alpha", "change_bps": "many"}`
	dropTable := func(table string) func(*testing.T, renamer, ids.CabalID) {
		return func(t *testing.T, r renamer, _ ids.CabalID) {
			t.Helper()
			r.exec(t, "ALTER TABLE "+table+" RENAME TO "+table+"_gone")
		}
	}
	refuseTitle := func(word string) func(*testing.T, renamer, ids.CabalID) {
		return func(t *testing.T, r renamer, _ ids.CabalID) {
			t.Helper()
			r.exec(t, "ALTER TABLE feed_objects ADD CONSTRAINT refuse CHECK (title NOT LIKE '%"+word+"%')")
		}
	}
	badRow := func(t *testing.T, r renamer, cabal ids.CabalID) {
		t.Helper()
		r.insertRaw(t, r.alice, cabal, badPayload)
	}
	identityDown := func(t *testing.T, r renamer, _ ids.CabalID) {
		t.Helper()
		r.users.Fail("UsersByID", errs.New(errs.CodeDBUnavailable, "test"))
	}
	cabalDown := func(t *testing.T, r renamer, _ ids.CabalID) {
		t.Helper()
		r.names.err = errs.New(errs.CodeDBUnavailable, "test")
	}
	for name, tc := range map[string]struct {
		handler string
		prepare func(*testing.T, renamer, ids.CabalID)
		code    errs.Code
	}{
		"profile identity": {profileHandler, identityDown, errs.CodeDBUnavailable},
		"profile select":   {profileHandler, dropTable("feed_objects"), errs.CodeInternal},
		"profile payload":  {profileHandler, badRow, errs.CodeInternal},
		"profile update":   {profileHandler, refuseTitle("Quillen"), errs.CodeInternal},
		"cabal read":       {cabalHandler, cabalDown, errs.CodeDBUnavailable},
		"cabal rename":     {cabalHandler, dropTable("feed_cabals"), errs.CodeInternal},
		"cabal select":     {cabalHandler, dropTable("feed_objects"), errs.CodeInternal},
		"cabal payload":    {cabalHandler, badRow, errs.CodeInternal},
		"cabal update":     {cabalHandler, refuseTitle("Quorum"), errs.CodeInternal},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newRenamer(t)
			r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice", DisplayName: "Quillen"})
			cabal := ids.CabalIDFrom(r.gen.NewV7())
			r.exec(
				t,
				"INSERT INTO feed_cabals (cabal_id, name, updated_at) VALUES ('"+cabal.String()+"', 'Alpha', now())",
			)
			r.seed(t, 1, joined(r.alice, cabal, "alice"))
			tc.prepare(t, r, cabal)

			var ev events.Event = profileUpdated(r.alice.UUID(), "Quillen", "display_name")
			if tc.handler == cabalHandler {
				ev = r.renamed(cabal, r.alice, "Quorum")
			}
			_, err := r.deliver(t, tc.handler, ev)

			wantCode(t, err, tc.code)
		})
	}
}

func TestFeedRename_keepsAConcurrentWritersChange(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice", DisplayName: "Quillen"})
	row := r.seed(t, 1, withChange(joined(r.alice, ids.CabalIDFrom(r.gen.NewV7()), "alice"), 2))[0]
	ev := profileUpdated(r.alice.UUID(), "Quillen", "display_name")
	id := r.event(t, ev)
	locked, release := make(chan struct{}), make(chan struct{})
	var writer, rename errgroup.Group
	writer.Go(func() error {
		return r.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			_, err := tx.Queries().
				Exec(ctx, `UPDATE feed_objects SET payload = payload || '{"change_bps": 3}' WHERE id = $1`, row)
			close(locked)
			<-release
			return err
		})
	})
	<-locked

	rename.Go(func() error {
		_, err := r.deliverAs(t.Context(), t, id, profileHandler, ev)
		return err
	})
	testkit.Eventually(t, func() bool { return r.lockWaiters(t) > 0 }, 10*time.Second)
	close(release)

	if err := writer.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := rename.Wait(); err != nil {
		t.Fatal(err)
	}
	var name string
	var voters int
	err := r.pool.QueryRow(
		t.Context(),
		`SELECT payload->>'actor_name', (payload->>'change_bps')::int FROM feed_objects WHERE id = $1`,
		row,
	).Scan(&name, &voters)
	if err != nil || name != "Quillen" || voters != 3 {
		t.Fatalf("row = %q with %d voters, %v; want Quillen with the concurrent writer's 3", name, voters, err)
	}
}

func (r renamer) lockWaiters(t *testing.T) int {
	t.Helper()
	var n int
	err := r.pool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database() AND wait_event_type = 'Lock'`,
	).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r renamer) insertRaw(t *testing.T, actor ids.UserID, cabal ids.CabalID, payload string) {
	t.Helper()
	_, err := r.pool.Exec(t.Context(), `INSERT INTO feed_objects
		(id, kind, ref_type, ref_id, cabal_id, cabal_name, actor_id, title, payload, created_at, updated_at)
		VALUES ($1, 'member_joined', 'cabal_members', $2, $3, 'Alpha', $4, 'alice joined Alpha', $5::jsonb, $6, $6)`,
		r.gen.NewV7(), r.gen.NewV7(), cabal.UUID(), actor.UUID(), payload, r.clock.Now())
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeedRename_CabalConvergesWhenRenamesArriveOutOfOrder(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice"})
	cabal := ids.CabalIDFrom(r.gen.NewV7())
	r.mustDeliver(t, createdHandler, events.CabalCreated{
		V: 1, CabalID: cabal.UUID(), CreatorID: r.alice.UUID(), Name: "Alpha",
	})
	r.seed(t, 3, joined(r.bob, cabal, "bob"))
	first := r.renamed(cabal, r.alice, "Beta")
	second := r.renamed(cabal, r.alice, "Quorum")

	r.mustDeliver(t, cabalHandler, second)
	r.mustDeliver(t, cabalHandler, first)

	got := r.count(t, `cabal_id = $1 AND cabal_name = 'Quorum' AND payload->>'cabal_name' = 'Quorum'`, cabal.UUID())
	if got != 4 {
		t.Fatalf("rows on the current name = %d, want the created item and bob's 3", got)
	}
	var name string
	if err := r.pool.QueryRow(t.Context(), `SELECT name FROM feed_cabals WHERE cabal_id = $1`, cabal.UUID()).
		Scan(&name); err != nil || name != "Quorum" {
		t.Fatalf("feed_cabals name = %q, %v; want Quorum", name, err)
	}
}

func TestFeedRename_ProfileRefreshesRowsWithNoStoredName(t *testing.T) {
	t.Parallel()
	r := newRenamer(t)
	r = r.knowing(identity.UserCard{ID: r.alice, Handle: "alice"})
	cabal := ids.CabalIDFrom(r.gen.NewV7())
	r.seed(t, 2, joined(r.alice, cabal, ""))
	if got := r.count(t, `actor_id = $1 AND NOT payload ? 'actor_name'`, r.alice.UUID()); got != 2 {
		t.Fatalf("seeded rows with no actor_name key = %d, want 2", got)
	}

	r.mustDeliver(t, profileHandler, profileUpdated(r.alice.UUID(), "", "handle"))

	if got := r.count(t, `actor_id = $1 AND title = 'alice joined Alpha' AND payload->>'actor_name' = 'alice'`,
		r.alice.UUID()); got != 2 {
		t.Fatalf("refreshed rows = %d, want 2", got)
	}
}
