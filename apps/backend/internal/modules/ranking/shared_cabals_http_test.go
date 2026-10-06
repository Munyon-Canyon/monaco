package ranking_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apibase "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type sharedWorld struct {
	viewer, other, third testkit.SeededUser
	shared, mine, theirs testkit.SeededCabal
}

func newSharedWorld(t *testing.T, s server) sharedWorld {
	t.Helper()
	w := sharedWorld{
		viewer: testkit.SeedUser(t, s.pool, testkit.UserOpts{}),
		other:  testkit.SeedUser(t, s.pool, testkit.UserOpts{}),
		third:  testkit.SeedUser(t, s.pool, testkit.UserOpts{}),
	}
	w.shared = testkit.NewCabal(t, s.pool, testkit.WithName("Shared"),
		testkit.WithCreator(w.viewer.ID), testkit.WithJoiner(w.other.ID))
	w.mine = testkit.NewCabal(t, s.pool, testkit.WithName("Mine"), testkit.WithCreator(w.viewer.ID))
	w.theirs = testkit.NewCabal(t, s.pool, testkit.WithName("Theirs"),
		testkit.WithCreator(w.other.ID), testkit.WithJoiner(w.third.ID))
	return w
}

func (w sharedWorld) fund(t *testing.T, s server, now time.Time) {
	t.Helper()
	ledger := testkit.NewLedger(t, s.pool)
	ledger.WithFundedMember(w.viewer.ID, w.shared.ID, money.MicrosFromUint64(100_000_000))
	ledger.WithFundedMember(w.other.ID, w.shared.ID, money.MicrosFromUint64(23_456_789))
	exec(t, s.pool, `UPDATE user_txns SET created_at = $2 WHERE cabal_id = $1`, w.shared.ID.UUID(), now.Add(-time.Hour))
	exec(t, s.pool, latestSnapshot, w.shared.ID.UUID(), now, 150_000_000, sharesOf(t, s, w.shared.ID))
	exec(t, s.pool, latestSnapshot, w.mine.ID.UUID(), now, 7_000_000, 1)
	exec(t, s.pool, latestSnapshot, w.theirs.ID.UUID(), now, 8_000_000, 1)
}

func sharedWith(t *testing.T, s server, viewer, other ids.UserID) *httptest.ResponseRecorder {
	t.Helper()
	return s.get(t, "/v1/users/"+other.String()+"/shared-cabals", viewer)
}

func TestSharedCabals_OnlyShared(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newSharedWorld(t, s)
	w.fund(t, s, s.clock.Now().UTC())
	seedRun(t, s)
	rec := sharedWith(t, s, w.viewer.ID, w.other.ID)
	var got api.SharedCabals
	decode(t, rec, &got)
	if rec.Code != http.StatusOK || len(got.Cabals) != 1 {
		t.Fatalf("GET = %d %s, want only the cabal both are in", rec.Code, rec.Body)
	}
	row := got.Cabals[0]
	if row.Cabal.Id != w.shared.ID.UUID() || row.Cabal.Name != "Shared" || row.ValueMicros != 150_000_000 ||
		row.PnlMicros != 150_000_000-123_456_789 || row.ReturnBps == nil || *row.ReturnBps != 2150 {
		t.Fatalf("row = %+v, want the pot's own value, P&L and return", row)
	}
	if rec = sharedWith(
		t,
		s,
		w.viewer.ID,
		w.third.ID,
	); rec.Code != http.StatusOK ||
		strings.Contains(rec.Body.String(), "Shared") {
		t.Fatalf("with a user who shares nothing = %d %s, want no cabal", rec.Code, rec.Body)
	}
}

func TestSharedCabals_NoStakeLeak(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newSharedWorld(t, s)
	w.fund(t, s, s.clock.Now().UTC())
	seedRun(t, s)
	rec := sharedWith(t, s, w.viewer.ID, w.other.ID)
	var raw struct {
		Cabals []map[string]any `json:"cabals"`
	}
	decode(t, rec, &raw)
	if len(raw.Cabals) != 1 || len(raw.Cabals[0]) != 4 {
		t.Fatalf("row keys = %v, want cabal, value_micros, pnl_micros and return_bps only", raw.Cabals)
	}
	for _, leak := range []string{"23456789", "share_units", "net_contributed", "stake", w.other.ID.String()} {
		if strings.Contains(rec.Body.String(), leak) {
			t.Fatalf("response holds %q: %s", leak, rec.Body)
		}
	}
}

func TestSharedCabals_RejectionsAndEmpty(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newSharedWorld(t, s)
	if rec := sharedWith(t, s, w.viewer.ID, ids.UserIDFrom(ids.Real{}.NewV7())); rec.Code != http.StatusNotFound ||
		problemOf(t, rec) != apibase.UserNotFound {
		t.Fatalf("unknown user = %d %s, want 404 user_not_found", rec.Code, rec.Body)
	}
	if rec := sharedWith(t, s, ids.UserID{}, w.other.ID); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d, want 401", rec.Code)
	}
	var got api.SharedCabals
	rec := sharedWith(t, s, w.viewer.ID, w.other.ID)
	decode(t, rec, &got)
	if rec.Code != http.StatusOK || got.Cabals == nil || len(got.Cabals) != 0 {
		t.Fatalf("before any snapshot = %d %s, want an empty list", rec.Code, rec.Body)
	}
}
