package identity_test

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/storage"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type purgeStore struct {
	app.PhotoStore
	mu      sync.Mutex
	deleted []ids.UserID
	err     error
	during  func()
}

func (s *purgeStore) DeleteAll(ctx context.Context, user ids.UserID) error {
	s.mu.Lock()
	s.deleted = append(s.deleted, user)
	s.mu.Unlock()
	if s.during != nil {
		s.during()
	}
	if s.err != nil {
		return s.err
	}
	if s.PhotoStore == nil {
		return nil
	}
	return s.PhotoStore.DeleteAll(ctx, user)
}

func (s *purgeStore) calls() []ids.UserID {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]ids.UserID(nil), s.deleted...)
}

type fakeBucket struct {
	photos storage.ProfilePhotos
	client *http.Client
}

func fakeSupabase(t *testing.T) fakeBucket {
	t.Helper()
	srv := httptest.NewServer(fakes.New())
	t.Cleanup(srv.Close)
	client, err := storage.New(config.Config{
		Supabase: config.Supabase{URL: srv.URL, ServiceRoleKey: "service-role"},
		Timeouts: config.Timeouts{Storage: time.Second},
	})
	if err != nil {
		t.Fatal(err)
	}
	return fakeBucket{photos: storage.ProfilePhotos{Storage: client}, client: srv.Client()}
}

func (b fakeBucket) put(t *testing.T, user ids.UserID) string {
	t.Helper()
	url, err := b.photos.Put(t.Context(), user.String()+"/a.png", "image/png", []byte("\x89PNG\r\n\x1a\nphoto"))
	if err != nil {
		t.Fatal(err)
	}
	return url
}

func (b fakeBucket) status(t *testing.T, url string) int {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := b.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func purgedAt(t *testing.T, f portFixture, user ids.UserID) *time.Time {
	t.Helper()
	var at *time.Time
	if err := f.pool.QueryRow(t.Context(), `SELECT photo_purged_at FROM users WHERE id = $1`,
		user.UUID()).Scan(&at); err != nil {
		t.Fatal(err)
	}
	return at
}

func expectTick(t *testing.T, p *app.PhotoPurges, scanned, changed int) {
	t.Helper()
	if r, err := p.Tick(t.Context()); err != nil || r.Scanned != scanned || r.Changed != changed {
		t.Fatalf("tick = %+v, %v, want %d scanned and %d changed", r, err, scanned, changed)
	}
}

func expectPurgedAt(t *testing.T, f portFixture, want time.Time, users ...ids.UserID) {
	t.Helper()
	for _, u := range users {
		if at := purgedAt(t, f, u); at == nil || !at.Equal(want) {
			t.Fatalf("photo_purged_at of %s = %v, want %s", u, at, want)
		}
	}
}

func TestPhotoPurges_deletesADeletedUsersPhotosOnceAndLeavesEveryoneElse(t *testing.T) {
	t.Parallel()
	f, clk := newPortFixture(t), testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	bucket := fakeSupabase(t)
	gone := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	noPhoto := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	active := testkit.SeedUser(t, f.pool, testkit.UserOpts{})
	goneURL, activeURL := bucket.put(t, gone.ID), bucket.put(t, active.ID)
	store := &purgeStore{PhotoStore: bucket.photos}
	p := app.NewPhotoPurges(f.pool, store, clk)
	if p.Name() != "identity.photo_purges" || p.Interval() != time.Minute {
		t.Fatalf("poller = %s every %s, want identity.photo_purges every 1m", p.Name(), p.Interval())
	}
	expectTick(t, p, 2, 2)
	if gonePhoto, activePhoto := bucket.status(
		t,
		goneURL,
	), bucket.status(
		t,
		activeURL,
	); gonePhoto != http.StatusNotFound ||
		activePhoto != http.StatusOK {
		t.Fatalf("photos after the tick: deleted user %d, active user %d, want 404 and 200", gonePhoto, activePhoto)
	}
	expectPurgedAt(t, f, clk.Now(), gone.ID, noPhoto.ID)
	if purgedAt(t, f, active.ID) != nil {
		t.Fatal("the active user was marked purged")
	}
	before := len(store.calls())
	expectTick(t, p, 0, 0)
	if after := len(store.calls()); after != before {
		t.Fatalf("the second tick deleted %d more prefixes, want none", after-before)
	}
}

func TestPhotoPurges_aStorageErrorLeavesTheRowAndWarnsOnce(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	gone := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	store := &purgeStore{err: errs.New(errs.CodeStorageUnavailable, "test.storage")}
	r, err := app.NewPhotoPurges(f.pool, store, testkit.NewClock(f.now)).Tick(ctx)
	if err != nil || r.Scanned != 1 || r.Changed != 0 {
		t.Fatalf("tick = %+v, %v, want 1 scanned and 0 changed", r, err)
	}
	if purgedAt(t, f, gone.ID) != nil {
		t.Fatal("a failed purge was marked purged")
	}
	if n := bytes.Count(logs.Bytes(), []byte("identity.photo_purge.failed")); n != 1 ||
		!strings.Contains(string(logs.Bytes()), gone.ID.String()) {
		t.Fatalf("logs = %s, want one identity.photo_purge.failed naming the user", logs.Bytes())
	}
}

func TestPhotoPurges_returnsTheDatabaseError(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := app.NewPhotoPurges(f.pool, &purgeStore{}, testkit.NewClock(f.now)).Tick(canceled); err == nil {
		t.Fatal("tick on a canceled context succeeded")
	}
	ctx, stop := context.WithCancel(t.Context())
	r, err := app.NewPhotoPurges(f.pool, &purgeStore{during: stop}, testkit.NewClock(f.now)).Tick(ctx)
	if err == nil || r.Scanned != 1 || r.Changed != 0 {
		t.Fatalf("tick that loses its context mid-row = %+v, %v, want the mark to fail", r, err)
	}
}

func TestModule_pollsWithTheInjectedStoreThenTheDepsStoreAndNeedsOne(t *testing.T) {
	t.Parallel()
	f := newPortFixture(t)
	gone := testkit.SeedUser(t, f.pool, testkit.UserOpts{AccountStatus: "deleted"})
	clk := testkit.NewClock(f.now)
	injected, fromDeps := &purgeStore{err: errs.New(errs.CodeStorageUnavailable, "t")}, &purgeStore{
		err: errs.New(errs.CodeStorageUnavailable, "t"),
	}
	deps := module.Deps{Pool: f.pool, Clock: clk, Photos: fromDeps}
	for _, tc := range []struct {
		name string
		mod  *identity.Module
		hit  *purgeStore
	}{
		{"an injected store wins", identity.New(deps, identity.WithPhotoStore(injected)), injected},
		{"the deps store is next", identity.New(deps), fromDeps},
	} {
		pollers := tc.mod.Pollers()
		if len(pollers) != 1 {
			t.Fatalf("%s: %d pollers, want 1", tc.name, len(pollers))
		}
		if _, err := pollers[0].Tick(t.Context()); err != nil {
			t.Fatal(err)
		}
		if got := tc.hit.calls(); len(got) != 1 || got[0] != gone.ID {
			t.Fatalf("%s: store calls = %v, want one for %s", tc.name, got, gone.ID)
		}
	}
	if got := identity.New(module.Deps{Pool: f.pool, Clock: clk}).Pollers(); got != nil {
		t.Fatalf("pollers without a photo store = %v, want none", got)
	}
}
