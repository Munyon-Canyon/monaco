package cabal_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/sqlc"
	api "github.com/monaco/monaco/apps/backend/internal/platform/httpx/api/cabalapi"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (f accessFixture) reorder(t *testing.T, user ids.UserID, order ...ids.CabalID) error {
	t.Helper()
	cmd := app.ReorderCabals{ActorID: user, CabalIDs: order}
	return app.NewReorderCabalsHandler(f.uow).Handle(as(t.Context(), user), cmd)
}

func (f accessFixture) order(t *testing.T, user ids.UserID) []ids.CabalID {
	t.Helper()
	rows, err := sqlc.New(f.pool).ListCabalIDsForUser(t.Context(), user.UUID())
	if err != nil {
		t.Fatal(err)
	}
	out := make([]ids.CabalID, len(rows))
	for i, id := range rows {
		out[i] = ids.CabalIDFrom(id)
	}
	return out
}

func (f accessFixture) cabalsOf(t *testing.T, user ids.UserID, n int) []ids.CabalID {
	t.Helper()
	out := make([]ids.CabalID, n)
	for i := range out {
		out[i] = testkit.NewCabal(t, f.pool, testkit.WithCreator(user)).ID
	}
	return out
}

func TestReorderCabals_readsBackInTheRequestedOrderAndAJoinedCabalComesLast(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user := f.user(t)
	c := f.cabalsOf(t, user, 3)
	if got := f.order(t, user); !slices.Equal(got, c) {
		t.Fatalf("order before = %v, want join order %v", got, c)
	}
	if err := f.reorder(t, user, c[2], c[0], c[1]); err != nil {
		t.Fatal(err)
	}
	if got, want := f.order(t, user), []ids.CabalID{c[2], c[0], c[1]}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	fourth := f.cabalsOf(t, user, 1)[0]
	if got, want := f.order(t, user), []ids.CabalID{c[2], c[0], c[1], fourth}; !slices.Equal(got, want) {
		t.Fatalf("order after a join = %v, want %v", got, want)
	}
}

func TestReorderCabals_aPartialOrderPutsTheListedFirstAndKeepsTheRest(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user := f.user(t)
	c := f.cabalsOf(t, user, 3)
	if err := f.reorder(t, user, c[2], c[1]); err != nil {
		t.Fatal(err)
	}
	if got, want := f.order(t, user), []ids.CabalID{c[2], c[1], c[0]}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	var distinct int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(DISTINCT sort_index) FROM cabal_members WHERE user_id = $1
		AND sort_index IS NOT NULL`, user.UUID()).Scan(&distinct); err != nil || distinct != 3 {
		t.Fatalf("distinct indexes = %d, %v; want 3", distinct, err)
	}
}

func TestReorderCabals_ignoresACabalTheCallerLeft(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user := f.user(t)
	c := f.cabalsOf(t, user, 3)
	f.exec(t, `DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`, c[0].UUID(), user.UUID())
	if err := f.reorder(t, user, c[0], c[2], c[1]); err != nil {
		t.Fatal(err)
	}
	if got, want := f.order(t, user), []ids.CabalID{c[2], c[1]}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestReorderCabals_refusesADuplicate(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user := f.user(t)
	c := f.cabalsOf(t, user, 2)
	if err := f.reorder(t, user, c[0], c[0]); errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("reorder = %v, want invalid_input", err)
	}
	if got := f.order(t, user); !slices.Equal(got, c) {
		t.Fatalf("order = %v, want it unchanged %v", got, c)
	}
}

func TestReorderCabals_leavesAnotherMembersOrderAlone(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	user, other := f.user(t), f.user(t)
	a := testkit.NewCabal(t, f.pool, testkit.WithCreator(user), testkit.WithJoiner(other))
	b := testkit.NewCabal(t, f.pool, testkit.WithCreator(user), testkit.WithJoiner(other))
	if err := f.reorder(t, user, b.ID, a.ID); err != nil {
		t.Fatal(err)
	}
	if got, want := f.order(t, other), []ids.CabalID{a.ID, b.ID}; !slices.Equal(got, want) {
		t.Fatalf("other's order = %v, want %v", got, want)
	}
}

func TestReorderCabals_wrapsABrokenMemberTable(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		before string
	}{
		{name: "lock", before: `ALTER TABLE cabal_members RENAME TO members_gone`},
		{name: "write", before: `CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN RAISE EXCEPTION 'refused'; END $$;
			CREATE TRIGGER refuse BEFORE UPDATE ON cabal_members FOR EACH ROW EXECUTE FUNCTION refuse()`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			user := f.user(t)
			c := f.cabalsOf(t, user, 2)
			f.exec(t, tt.before)
			wantErr(t, f.reorder(t, user, c[1], c[0]), errs.CodeInternal)
		})
	}
}

func TestPutMyCabalOrder_refusesNoCallerAndNoBody(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	routes := adapters.HTTP{Reorder: app.NewReorderCabalsHandler(f.uow)}
	_, err := routes.PutMyCabalOrder(t.Context(), api.PutMyCabalOrderRequestObject{})
	wantErr(t, err, errs.CodeUnauthorized)
	_, err = routes.PutMyCabalOrder(as(t.Context(), f.user(t)), api.PutMyCabalOrderRequestObject{})
	wantErr(t, err, errs.CodeInvalidInput)
}
