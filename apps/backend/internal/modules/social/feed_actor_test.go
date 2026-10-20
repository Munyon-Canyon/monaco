package social_test

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	tracenoop "go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/modules/social"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain/feed"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/socialapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f feedFixture) byActor(t *testing.T, actor ids.UserID, cabal ids.CabalID, kind feed.Kind) uuid.UUID {
	t.Helper()
	return f.item(t, func(it *feed.Item) { it.Kind, it.CabalID, it.ActorID = kind, cabal, actor })
}

func feedIDs(page api.FeedPage) []uuid.UUID {
	out := make([]uuid.UUID, len(page.Items))
	for i, it := range page.Items {
		out[i] = it.Id
	}
	return out
}

func TestGetFeed_actorIDListsOnlyThatUsersItemsNewestFirstAcrossPages(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, a, b := ids.NewUserID(f.gen), ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	cabal := ids.CabalIDFrom(f.gen.NewV7())
	mine := make([]uuid.UUID, 0, 3)
	for range 3 {
		mine = append(mine, f.item(t, func(it *feed.Item) { it.CabalID, it.ActorID = cabal, a }))
		f.item(t, func(it *feed.Item) { it.CabalID, it.ActorID = cabal, b })
		f.item(t, func(it *feed.Item) { it.Kind = feed.KindPriceMove })
		f.clock.Advance(time.Second)
	}
	slices.Reverse(mine)
	first := f.getFeed(t, viewer, api.GetFeedParams{ActorId: ptr(a.UUID()), Limit: ptr(2)})
	if first.NextCursor == nil || !slices.Equal(feedIDs(first), mine[:2]) {
		t.Fatalf("first page = %v, cursor %v, want %v and a cursor", feedIDs(first), first.NextCursor, mine[:2])
	}
	second := f.getFeed(t, viewer, api.GetFeedParams{ActorId: ptr(a.UUID()), Limit: ptr(2), Cursor: first.NextCursor})
	if second.NextCursor != nil || !slices.Equal(feedIDs(second), mine[2:]) {
		t.Fatalf("second page = %v, cursor %v, want %v and no cursor", feedIDs(second), second.NextCursor, mine[2:])
	}
	top := f.getFeed(t, viewer, api.GetFeedParams{ActorId: ptr(a.UUID()), Sort: ptr(api.GetFeedParamsSort("top"))})
	if !sameSet(feedIDs(top), mine) {
		t.Fatalf("top page = %v, want %v", feedIDs(top), mine)
	}
}

func TestGetFeed_actorIDCombinesWithKindScopeAndHiddenCabals(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, a := ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	alpha, bravo := ids.CabalIDFrom(f.gen.NewV7()), ids.CabalIDFrom(f.gen.NewV7())
	banned := ids.CabalIDFrom(f.gen.NewV7())
	f.membership(t, alpha, viewer, true)
	proposal := f.byActor(t, a, alpha, feed.KindProposal)
	joined := f.byActor(t, a, alpha, feed.KindMemberJoined)
	created := f.byActor(t, a, alpha, feed.KindCabalCreated)
	elsewhere := f.byActor(t, a, bravo, feed.KindProposal)
	gone := f.byActor(t, a, banned, feed.KindProposal)
	system := f.item(t, func(it *feed.Item) { it.CabalID = alpha })
	if err := f.ban(t, banned); err != nil {
		t.Fatal(err)
	}
	for name, tt := range map[string]struct {
		params api.GetFeedParams
		want   []uuid.UUID
	}{
		"actor":          {api.GetFeedParams{}, []uuid.UUID{proposal, joined, created, elsewhere}},
		"actor and kind": {api.GetFeedParams{Kind: ptr("proposal")}, []uuid.UUID{proposal, elsewhere}},
		"actor and scope": {
			api.GetFeedParams{Scope: ptr(api.GetFeedParamsScope("mine"))}, []uuid.UUID{proposal, joined, created},
		},
	} {
		tt.params.ActorId = ptr(a.UUID())
		if got := feedIDs(f.getFeed(t, viewer, tt.params)); !sameSet(got, tt.want) {
			t.Errorf("%s: got %v, want %v", name, got, tt.want)
		}
	}
	every := []uuid.UUID{proposal, joined, created, elsewhere, gone, system}
	shown := []uuid.UUID{proposal, joined, created, elsewhere}
	f.wantVisibleExactly(t, viewer, app.FeedFilter{ActorID: a}, every, shown)
}

func TestGetFeedItem_actorIDTellsWhetherTheItemBelongsToThatUser(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	viewer, a, b := ids.NewUserID(f.gen), ids.NewUserID(f.gen), ids.NewUserID(f.gen)
	id := f.byActor(t, a, ids.CabalIDFrom(f.gen.NewV7()), feed.KindProposal)
	for actor, visible := range map[ids.UserID]bool{a: true, b: false} {
		res, err := f.routes().GetFeedItem(asUser(t.Context(), viewer), api.GetFeedItemRequestObject{
			Id: id, Params: api.GetFeedItemParams{ActorId: ptr(actor.UUID())},
		})
		if err != nil {
			t.Fatal(err)
		}
		got, ok := res.(api.GetFeedItem200JSONResponse)
		if !ok || got.Item.Id != id || got.Visible != visible {
			t.Fatalf("actor_id=%s: response = %+v, want the item with visible %v", actor, res, visible)
		}
	}
}

func TestGetFeed_aMalformedActorIDIsInvalidInput(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	verifier, err := auth.NewDevVerifier(
		config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "test-only"}}, f.clock)
	if err != nil {
		t.Fatal(err)
	}
	deps := module.Deps{Pool: f.pool, UoW: db.New(f.pool, f.gen, f.clock), IDs: f.gen, Clock: f.clock}
	h, err := httpx.HandlerFor(httpx.Deps{
		Logger:       observability.NewLogger(config.Config{Env: config.EnvTest}, &testkit.Logs{}),
		Tracer:       tracenoop.NewTracerProvider(),
		Clock:        f.clock,
		IDs:          f.gen,
		MaxBodyBytes: 1 << 20,
		Idempotency:  db.NewIdempotencyStore(f.pool, f.clock),
		Verifier:     verifier,
	}, social.New(deps).Mount, socialContract(t))
	if err != nil {
		t.Fatal(err)
	}
	h = testkit.HTTP(t, h)
	token := verifier.Mint(ids.NewUserID(f.gen).String(), f.clock.Now().Add(time.Hour))
	for _, path := range []string{"/v1/feed?actor_id=nope", "/v1/feed/" + f.gen.NewV7().String() + "?actor_id=nope"} {
		req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "invalid_input") {
			t.Errorf("GET %s = %d %s, want 400 invalid_input", path, rec.Code, rec.Body)
		}
	}
}

func TestFeedQuery_actorOnlyFilterReadsTheActorIndex(t *testing.T) {
	t.Parallel()
	f := newFeedFixture(t)
	a := ids.NewUserID(f.gen)
	_, err := f.pool.Exec(t.Context(), `INSERT INTO feed_objects (id, kind, ref_type, ref_id, actor_id, title, payload,
		created_at, updated_at)
		SELECT gen_random_uuid(), 'trade', 'swaps', gen_random_uuid(),
			CASE WHEN n % 500 = 0 THEN $2::uuid ELSE gen_random_uuid() END,
			'Cabal bought $' || n || ' of SYM', '{}', $1::timestamptz - n * interval '1 second', $1
		FROM generate_series(1, 10000) AS n`, f.clock.Now(), a.UUID())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `ANALYZE feed_objects`); err != nil {
		t.Fatal(err)
	}
	traced, last := f.traced(t)
	page, err := app.ListFeed(t.Context(), traced, app.FeedQuery{
		Viewer: ids.NewUserID(f.gen), Filter: app.FeedFilter{ActorID: a}, Limit: app.FeedPageMax,
	})
	if err != nil || len(page.Items) != 20 {
		t.Fatalf("actor page = %d items, %v, want 20", len(page.Items), err)
	}
	rows, err := f.pool.Query(t.Context(), "EXPLAIN "+last.sql, last.args...)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(plan, "\n")
	if strings.Contains(text, "Seq Scan on feed_objects") || !strings.Contains(text, "feed_objects_actor_created_idx") {
		t.Fatalf("actor feed page plan:\n%s\nwant feed_objects_actor_created_idx and no seq scan of feed_objects", text)
	}
}
