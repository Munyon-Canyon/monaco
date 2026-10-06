package social_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/social/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f contactFixture) list(t *testing.T, user ids.UserID, after *domain.Keyset, limit int) app.ContactMatchPage {
	t.Helper()
	page, err := app.ListContactMatches(f.ctx(t, nil), f.pool, f.readers, app.ContactMatchQuery{
		User: user, After: after, Limit: limit,
	})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestContactMatches_FollowedByMe(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "follower"})
	them := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "friend"})
	_, err := f.pool.Exec(t.Context(), `UPDATE users SET display_name = 'Friend One', photo_url = $2 WHERE id = $1`,
		them.ID.UUID(), "https://img.example/f.png")
	if err != nil {
		t.Fatal(err)
	}
	setPhone(t, f.pool, them.ID, "+14155552001", true)
	hashes := []string{contactHash("+14155552001")}
	if err := f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: hashes}); err != nil {
		t.Fatal(err)
	}
	want := app.ContactMatch{
		UserID: them.ID, Handle: "friend", DisplayName: "Friend One", PhotoURL: "https://img.example/f.png",
	}
	requireUnpagedContact(t, f.list(t, me.ID, nil, app.ContactPageDefault), want)
	if err := f.followOn.Handle(f.ctx(t, nil), app.Follow{
		Follower: me.ID, Followee: them.ID, Source: domain.SourcePhone,
	}); err != nil {
		t.Fatal(err)
	}
	want.FollowedByMe = true
	requireUnpagedContact(t, f.list(t, me.ID, nil, app.ContactPageDefault), want)
	if err := f.unfollow.Handle(f.ctx(t, nil), app.Unfollow{Follower: me.ID, Followee: them.ID}); err != nil {
		t.Fatal(err)
	}
	want.FollowedByMe = false
	requireUnpagedContact(t, f.list(t, me.ID, nil, app.ContactPageDefault), want)
}

func TestListContactMatches_pagesAndDropsPeopleWhoLeft(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "pager"})
	first := seedPhoneUser(t, f.pool, "page_a", "+14155553001", "active", true)
	second := seedPhoneUser(t, f.pool, "page_b", "+14155553002", "active", true)
	held := seedPhoneUser(t, f.pool, "page_c", "+14155553003", "active", true)
	hashes := []string{contactHash("+14155553001"), contactHash("+14155553002"), contactHash("+14155553003")}
	if err := f.match.Handle(f.ctx(t, nil), app.MatchContacts{User: me.ID, Hashes: hashes}); err != nil {
		t.Fatal(err)
	}
	page := f.list(t, me.ID, nil, 1)
	if len(page.Items) != 1 || page.Next == nil {
		t.Fatalf("first page = %+v, want one item and a cursor", page)
	}
	rest := f.list(t, me.ID, page.Next, 30)
	if len(rest.Items) != 2 || rest.Next != nil {
		t.Fatalf("second page = %+v, want the other two", rest)
	}
	setAccountStatus(t, f.pool, first, "banned")
	setAccountStatus(t, f.pool, second, "suspended")
	if _, err := f.pool.Exec(t.Context(), `UPDATE users SET account_status = 'deleted', deleted_at = $2 WHERE id = $1`,
		held.UUID(), f.now); err != nil {
		t.Fatal(err)
	}
	page = f.list(t, me.ID, nil, app.ContactPageDefault)
	if len(page.Items) != 1 || page.Items[0].UserID != second {
		t.Fatalf("visible = %+v, want only the suspended user", page.Items)
	}
	assertDirectoryAndLimitErrors(t, f, me.ID)
}

func TestListContactMatches_anEmptyPageHasNoCursor(t *testing.T) {
	t.Parallel()
	f := newContactFixture(t)
	me := testkit.SeedUser(t, f.pool, testkit.UserOpts{Handle: "empty_pg"})
	page := f.list(t, me.ID, nil, app.ContactPageMax)
	if len(page.Items) != 0 || page.Next != nil {
		t.Fatalf("page = %+v, want empty", page)
	}
}

func requireUnpagedContact(t *testing.T, page app.ContactMatchPage, want app.ContactMatch) {
	t.Helper()
	if len(page.Items) != 1 || page.Next != nil {
		t.Fatalf("page = %+v, want one item and no cursor", page)
	}
	if page.Items[0] != want {
		t.Fatalf("item = %+v, want %+v", page.Items[0], want)
	}
}

func setAccountStatus(t *testing.T, pool *pgxpool.Pool, id ids.UserID, status string) {
	t.Helper()
	_, err := pool.Exec(t.Context(), `UPDATE users SET account_status = $2 WHERE id = $1`, id.UUID(), status)
	if err != nil {
		t.Fatal(err)
	}
}

func assertDirectoryAndLimitErrors(t *testing.T, f contactFixture, me ids.UserID) {
	t.Helper()
	hidden, err := app.ListContactMatches(f.ctx(t, nil), f.pool, hideDirectory{}, app.ContactMatchQuery{
		User: me, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hidden.Items) != 0 || hidden.Next == nil {
		t.Fatalf("hidden page = %+v, want no items and a cursor", hidden)
	}
	_, err = app.ListContactMatches(f.ctx(t, nil), f.pool, brokenDirectory{}, app.ContactMatchQuery{
		User: me, Limit: 1,
	})
	wantCode(t, err, errs.CodeInternal)
	for _, limit := range []int{0, app.ContactPageMax + 1} {
		_, err = app.ListContactMatches(f.ctx(t, nil), f.pool, f.readers, app.ContactMatchQuery{
			User: me, Limit: limit,
		})
		wantCode(t, err, errs.CodeInvalidInput)
	}
	if _, err := f.pool.Exec(t.Context(), `DROP TABLE contact_matches`); err != nil {
		t.Fatal(err)
	}
	_, err = app.ListContactMatches(f.ctx(t, nil), f.pool, f.readers, app.ContactMatchQuery{
		User: me, Limit: 1,
	})
	wantCode(t, err, errs.CodeInternal)
}

type hideDirectory struct{}

func (hideDirectory) UsersByID(context.Context, []ids.UserID) (map[ids.UserID]identity.UserCard, error) {
	return map[ids.UserID]identity.UserCard{}, nil
}

func (hideDirectory) UsersByPhoneHashes(context.Context, [][]byte) (map[string]ids.UserID, error) {
	return map[string]ids.UserID{}, nil
}

func (hideDirectory) UserIDsByHandles(context.Context, []string) (map[string]ids.UserID, error) {
	return map[string]ids.UserID{}, nil
}
