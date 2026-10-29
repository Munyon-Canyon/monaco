package cabal_test

import (
	"encoding/base64"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func membersPage(
	t *testing.T, f createFixture, cabalID ids.CabalID, limit *int, cursor *string,
) (api.CabalMemberPage, error) {
	t.Helper()
	ctx := auth.WithActor(t.Context(), auth.Actor{
		Kind: auth.ActorUser, ID: f.user.ID.String(), Standing: auth.StandingActive,
	})
	res, err := f.routes(nil).GetCabalMembers(ctx, api.GetCabalMembersRequestObject{
		Id: cabalID.UUID(), Params: api.GetCabalMembersParams{Limit: limit, Cursor: cursor},
	})
	if err != nil {
		return api.CabalMemberPage{}, err
	}
	page, ok := res.(api.GetCabalMembers200JSONResponse)
	if !ok {
		t.Fatalf("GetCabalMembers = %T", res)
	}
	return api.CabalMemberPage(page), nil
}

func joinOrder(t *testing.T, f createFixture, cabalID ids.CabalID) []uuid.UUID {
	t.Helper()
	rows, err := f.pool.Query(t.Context(),
		`SELECT user_id FROM cabal_members WHERE cabal_id = $1 ORDER BY joined_at, user_id`, cabalID.UUID())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var order []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		order = append(order, id)
	}
	return order
}

func walkMembers(t *testing.T, f createFixture, cabalID ids.CabalID, limit int) (members []uuid.UUID, pages int) {
	t.Helper()
	var cursor *string
	for {
		page, err := membersPage(t, f, cabalID, &limit, cursor)
		if err != nil {
			t.Fatal(err)
		}
		pages++
		for _, member := range page.Items {
			members = append(members, member.UserId)
		}
		if page.NextCursor == nil {
			return members, pages
		}
		cursor = page.NextCursor
	}
}

func TestGetCabalMembers_walksEveryMemberOnceInJoinOrder(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(120))
	tied := time.Date(2026, 10, 2, 15, 30, 0, 0, time.UTC)
	if _, err := f.pool.Exec(t.Context(),
		`UPDATE cabal_members SET joined_at = $2 WHERE cabal_id = $1 AND joined_at > $2`,
		seeded.ID.UUID(), tied); err != nil {
		t.Fatal(err)
	}
	got, pages := walkMembers(t, f, seeded.ID, 50)
	if pages != 3 || !slices.Equal(got, joinOrder(t, f, seeded.ID)) {
		t.Fatalf("pages %d, members %d", pages, len(got))
	}
	if len(slices.Compact(slices.Sorted(slices.Values(uuidStrings(got))))) != 120 {
		t.Fatal("a member appeared on two pages")
	}
}

func uuidStrings(values []uuid.UUID) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = value.String()
	}
	return out
}

func TestGetCabal_servesTheCountAndNoMemberLimitForALargeCabal(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(120))
	view := readCabal(t, f, seeded.Creator.ID, seeded.ID)
	if view.MemberCount != 120 || view.Me == nil {
		t.Fatalf("member count %d, me %+v", view.MemberCount, view.Me)
	}
}

func TestGetCabalMembers_carriesTheUserCardAndVoteStanding(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	setProfile(t, f, seeded.Creator.ID, "kai", "Kai Cenat", "https://cdn.example/kai.jpg")
	page, err := membersPage(t, f, seeded.ID, nil, nil)
	if err != nil || page.NextCursor != nil || len(page.Items) != 2 {
		t.Fatalf("page = %+v, %v", page, err)
	}
	first := page.Items[0]
	if first.UserId != seeded.Creator.ID.UUID() || first.Role != "creator" || !first.CanVote ||
		deref(first.Handle) != "kai" || first.DisplayName != "Kai Cenat" || deref(first.PhotoUrl) == "" {
		t.Fatalf("first = %+v", first)
	}
}

func TestGetCabalMembers_refusesABadLimitCursorAndMissingCabal(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	zero, over := 0, 201
	junk := "not-a-cursor!"
	for name, run := range map[string]func() error{
		"limit zero": func() error { _, err := membersPage(t, f, seeded.ID, &zero, nil); return err },
		"limit over": func() error { _, err := membersPage(t, f, seeded.ID, &over, nil); return err },
		"cursor":     func() error { _, err := membersPage(t, f, seeded.ID, nil, &junk); return err },
	} {
		if code := errs.CodeOf(run()); code != errs.CodeInvalidInput {
			t.Fatalf("%s: code %s", name, code)
		}
	}
	_, err := membersPage(t, f, ids.CabalIDFrom(f.ids.NewV7()), nil, nil)
	if errs.CodeOf(err) != errs.CodeCabalNotFound {
		t.Fatalf("missing cabal: %v", err)
	}
}

func TestListMembersPage_reportsBrokenReadsAndMalformedCursors(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	users := f.routes(nil).Users
	reads := app.MemberReads{Ownership: app.NoOwnership{}, Calls: app.NoCalls{}, Now: f.clock.Now}
	req := app.ListMembersQuery{CabalID: seeded.ID, Limit: 10, Order: app.OrderJoined}
	if _, err := app.ListMembersPage(
		t.Context(),
		f.pool,
		failCards{},
		reads,
		req,
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("card lookup failure = %v", err)
	}
	for name, raw := range map[string]string{
		"no separator": "nosep", "bad time": "abc\x00" + seeded.ID.String(), "bad id": "123\x00not-a-uuid",
	} {
		req.Cursor = base64.RawURLEncoding.EncodeToString([]byte(raw))
		if _, err := app.ListMembersPage(
			t.Context(),
			f.pool,
			users,
			reads,
			req,
		); errs.CodeOf(
			err,
		) != errs.CodeInvalidInput {
			t.Fatalf("%s: %v", name, err)
		}
	}
	req.Cursor = ""
	execSQL(t, f.pool, `ALTER TABLE cabal_members RENAME COLUMN role TO role_gone`)
	if _, err := app.ListMembersPage(t.Context(), f.pool, users, reads, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("members read failure = %v", err)
	}
	execSQL(t, f.pool, `ALTER TABLE cabals RENAME TO cabals_gone`)
	if _, err := app.ListMembersPage(t.Context(), f.pool, users, reads, req); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("cabal read failure = %v", err)
	}
}

func TestGetCabalMembers_rejectsACallerThatIsNotAUser(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	service := auth.WithActor(t.Context(), auth.Actor{Kind: auth.ActorService, ID: f.user.ID.String()})
	_, err := f.routes(nil).GetCabalMembers(service, api.GetCabalMembersRequestObject{Id: f.ids.NewV7()})
	if errs.CodeOf(err) != errs.CodeForbidden {
		t.Fatalf("service caller = %v", err)
	}
}

func TestGetCabalMembers_pastTheLastMemberIsAnEmptyPage(t *testing.T) {
	t.Parallel()
	f := newCreate(t)
	seeded := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	last := base64.RawURLEncoding.EncodeToString([]byte("9\x000\x009999999999999999\x00" + uuid.Max.String()))
	page, err := membersPage(t, f, seeded.ID, nil, &last)
	if err != nil || len(page.Items) != 0 || page.NextCursor != nil {
		t.Fatalf("page = %+v, %v", page, err)
	}
}
