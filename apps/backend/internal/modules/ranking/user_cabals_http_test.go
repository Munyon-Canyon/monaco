package ranking_test

import (
	"net/http"
	"testing"
	"time"

	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/rankingapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type userCabalsWorld struct {
	viewer, other ids.UserID
	shared        testkit.SeededCabal
	apart         testkit.SeededCabal
}

func newUserCabalsWorld(t *testing.T, s server) userCabalsWorld {
	t.Helper()
	shared := testkit.NewCabal(t, s.pool, testkit.WithName("Shared"), testkit.WithMembers(3))
	other, viewer := shared.Members[0].ID, shared.Members[1].ID
	apart := testkit.NewCabal(t, s.pool, testkit.WithName("Apart"), testkit.WithCreator(other))
	now := s.clock.Now().UTC()
	ledger := testkit.NewLedger(t, s.pool)
	for i, funds := range []uint64{35_000_000, 33_333_333, 31_666_667} {
		ledger.WithFundedMember(shared.Members[i].ID, shared.ID, money.MicrosFromUint64(funds))
		backdateFunds(t, s, shared.Members[i].ID, now.Add(-time.Hour))
	}
	exec(t, s.pool, latestSnapshot, shared.ID.UUID(), now, 100_000_000, sharesOf(t, s, shared.ID))
	seedRun(t, s)
	return userCabalsWorld{viewer: viewer, other: other, shared: shared, apart: apart}
}

func userCabalsOf(t *testing.T, s server, viewer, other ids.UserID) api.UserCabals {
	t.Helper()
	rec := s.get(t, "/v1/users/"+other.String()+"/cabals", viewer)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s, want 200", rec.Code, rec.Body)
	}
	var got api.UserCabals
	decode(t, rec, &got)
	return got
}

func TestUserCabals_ListsSharedFirstWithTheMembersOwnShare(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newUserCabalsWorld(t, s)
	exec(t, s.pool, `UPDATE users SET share_performance = true WHERE id = $1`, w.other.UUID())
	got := userCabalsOf(t, s, w.viewer, w.other)
	if len(got.Cabals) != 2 {
		t.Fatalf("cabals = %+v, want the shared one and the other's own", got.Cabals)
	}
	first := got.Cabals[0]
	if first.Cabal.Id != w.shared.ID.UUID() || !first.Shared || first.Role != api.UserCabalRoleAdmin ||
		!first.CanVote ||
		first.MemberCount != 3 {
		t.Errorf("first = %+v, want the shared cabal, admin, voting, 3 members", first)
	}
	if first.OwnershipBps == nil || *first.OwnershipBps != 3501 || *first.OwnershipBps != ownedIn(t, s, w) {
		t.Errorf("first share = %v, want the portfolio's 3501", first.OwnershipBps)
	}
}

func TestUserCabals_KeepsACabalWithoutAStake(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newUserCabalsWorld(t, s)
	exec(t, s.pool, `UPDATE users SET share_performance = true WHERE id = $1`, w.other.UUID())
	second := userCabalsOf(t, s, w.viewer, w.other).Cabals[1]
	if second.Cabal.Id != w.apart.ID.UUID() || second.Shared || second.OwnershipBps == nil ||
		*second.OwnershipBps != 0 {
		t.Errorf("second = %+v, want the unshared cabal with no stake kept at 0", second)
	}
}

func ownedIn(t *testing.T, s server, w userCabalsWorld) int32 {
	t.Helper()
	for _, row := range portfolioOf(t, s, w.other).Cabals {
		if row.Cabal.Id == w.shared.ID.UUID() {
			return row.OwnershipBps
		}
	}
	t.Fatal("shared cabal missing from the portfolio")
	return 0
}

func TestUserCabals_HidesTheShareOfAMemberWhoKeepsPerformancePrivate(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newUserCabalsWorld(t, s)
	exec(t, s.pool, `UPDATE users SET share_performance = false WHERE id = $1`, w.other.UUID())
	for _, row := range userCabalsOf(t, s, w.viewer, w.other).Cabals {
		if row.OwnershipBps != nil {
			t.Errorf("%s = %d, want null for another viewer", row.Cabal.Name, *row.OwnershipBps)
		}
	}
	own := userCabalsOf(t, s, w.other, w.other).Cabals
	if len(own) != 2 || own[0].OwnershipBps == nil || *own[0].OwnershipBps != 3501 {
		t.Errorf("own cabals = %+v, want the real 3501", own)
	}
}

func TestUserCabals_BlockEitherWayEmptiesTheList(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newUserCabalsWorld(t, s)
	block := func(blocker, blocked ids.UserID) {
		exec(t, s.pool, `INSERT INTO user_blocks (id, blocker_id, blocked_id, created_at) VALUES ($1, $2, $3, $4)`,
			ids.Real{}.NewV7(), blocker.UUID(), blocked.UUID(), s.clock.Now())
	}
	block(w.other, w.viewer)
	if got := userCabalsOf(t, s, w.viewer, w.other); got.Cabals == nil || len(got.Cabals) != 0 {
		t.Fatalf("blocked by the user: cabals = %+v, want empty", got.Cabals)
	}
	exec(t, s.pool, `DELETE FROM user_blocks`)
	block(w.viewer, w.other)
	if got := userCabalsOf(t, s, w.viewer, w.other); got.Cabals == nil || len(got.Cabals) != 0 {
		t.Fatalf("blocking the user: cabals = %+v, want empty", got.Cabals)
	}
}

func TestUserCabals_Rejections(t *testing.T) {
	t.Parallel()
	s := newServer(t)
	w := newUserCabalsWorld(t, s)
	unknown := ids.UserIDFrom(ids.Real{}.NewV7())
	if rec := s.get(t, "/v1/users/"+unknown.String()+"/cabals", w.viewer); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown user = %d %s, want 404", rec.Code, rec.Body)
	}
	if rec := s.get(t, "/v1/users/"+w.other.String()+"/cabals", ids.UserID{}); rec.Code != http.StatusUnauthorized {
		t.Fatalf("without a token = %d, want 401", rec.Code)
	}
}
