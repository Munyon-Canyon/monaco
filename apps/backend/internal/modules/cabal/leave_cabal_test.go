package cabal_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type treasuryStub struct {
	shares    money.SharesUnits
	pot       money.Micros
	sharesErr error
	potErr    error
	read      func()
	potReads  atomic.Int32
}

func (s *treasuryStub) ShareUnits(context.Context, ids.CabalID, ids.UserID) (money.SharesUnits, error) {
	if s.read != nil {
		s.read()
	}
	return s.shares, s.sharesErr
}

func (s *treasuryStub) PotValue(context.Context, ids.CabalID) (money.Micros, error) {
	s.potReads.Add(1)
	return s.pot, s.potErr
}

func (f accessFixture) leave(ctx context.Context, tr app.TreasuryReads, user ids.UserID, c ids.CabalID) error {
	return app.NewLeaveCabalHandler(f.uow, f.pool, tr).Handle(as(ctx, user), app.LeaveCabal{ActorID: user, CabalID: c})
}

func (f accessFixture) lefts(t *testing.T) []events.CabalMemberLeft {
	t.Helper()
	return decoded[events.CabalMemberLeft](t, f, events.TypeCabalMemberLeft)
}

func TestLeaveCabal_deletesTheMemberAndAppendsTheLeaveWithoutReadingThePot(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(3))
	leaver := c.Members[1].ID
	tr := &treasuryStub{pot: pot(9)}
	if err := f.leave(t.Context(), tr, leaver, c.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := f.membership(t, c.ID, leaver); ok {
		t.Fatal("the leaver is still a member")
	}
	want := events.CabalMemberLeft{V: 1, CabalID: c.ID.UUID(), UserID: leaver.UUID(), WasVoter: true}
	if got := f.lefts(t); len(got) != 1 || got[0] != want {
		t.Fatalf("member_left events = %+v, want %+v", got, want)
	}
	if n := tr.potReads.Load(); n != 0 {
		t.Fatalf("PotValue read %d times, want none while other members remain", n)
	}
}

func TestLeaveCabal_letsTheCreatorLeaveABannedCabalAsTheLastMemberOfAnEmptyPot(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool)
	f.exec(t, `UPDATE cabals SET status = 'banned' WHERE id = $1`, c.ID.UUID())
	if err := f.leave(t.Context(), &treasuryStub{}, c.Creator.ID, c.ID); err != nil {
		t.Fatal(err)
	}
	want := events.CabalMemberLeft{V: 1, CabalID: c.ID.UUID(), UserID: c.Creator.ID.UUID(), WasVoter: true}
	if got := f.lefts(t); len(got) != 1 || got[0] != want {
		t.Fatalf("member_left events = %+v, want %+v", got, want)
	}
	var members int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_members WHERE cabal_id = $1`,
		c.ID.UUID()).Scan(&members); err != nil || members != 0 {
		t.Fatalf("members = %d, %v; want an empty cabal", members, err)
	}
}

func TestLeaveCabal_refusesWithoutWritingAnything(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	pair := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	alone := testkit.NewCabal(t, f.pool)
	priceGone := errs.New(errs.CodePriceUnavailable, "test.PotValue")
	for _, tt := range []struct {
		name  string
		tr    *treasuryStub
		user  ids.UserID
		cabal ids.CabalID
		want  errs.Code
	}{
		{"an outsider", &treasuryStub{}, f.user(t), pair.ID, errs.CodeNotCabalMember},
		{"an unknown cabal", &treasuryStub{}, pair.Creator.ID, ids.CabalIDFrom(f.ids.NewV7()), errs.CodeNotCabalMember},
		{"a member holding shares", &treasuryStub{shares: shares(1)}, pair.Members[1].ID, pair.ID, errs.CodeLeaveHoldsShares},
		{
			"the last member of a full pot", &treasuryStub{pot: pot(1)}, alone.Creator.ID, alone.ID,
			errs.CodeLeaveLastMemberPotNotEmpty,
		},
		{"the creator with a member left", &treasuryStub{}, pair.Creator.ID, pair.ID, errs.CodeLeaveCreatorWithMembers},
		{"an unpriced pot", &treasuryStub{potErr: priceGone}, alone.Creator.ID, alone.ID, errs.CodePriceUnavailable},
		{"an unread stake", &treasuryStub{sharesErr: priceGone}, pair.Members[1].ID, pair.ID, errs.CodePriceUnavailable},
	} {
		if err := f.leave(t.Context(), tt.tr, tt.user, tt.cabal); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := f.lefts(t); len(got) != 0 {
		t.Fatalf("member_left events = %+v, want none", got)
	}
	if got := len(memberRows(t, f.pool, pair)) + len(memberRows(t, f.pool, alone)); got != 3 {
		t.Fatalf("members = %d, want all 3 still in", got)
	}
}

func TestLeaveCabal_rechecksTheMembersItLocks(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	for _, tt := range []struct {
		name string
		gone func(c testkit.SeededCabal) ids.UserID
		pot  money.Micros
		want errs.Code
	}{
		{
			"the leaver left first", func(c testkit.SeededCabal) ids.UserID { return c.Members[1].ID },
			pot(0), errs.CodeNotCabalMember,
		},
		{
			"the leaver became the last member", func(c testkit.SeededCabal) ids.UserID { return c.Creator.ID },
			pot(5), errs.CodeLeaveLastMemberPotNotEmpty,
		},
	} {
		c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
		gone := tt.gone(c)
		tr := &treasuryStub{pot: tt.pot, read: func() {
			f.exec(t, `DELETE FROM cabal_members WHERE cabal_id = $1 AND user_id = $2`, c.ID.UUID(), gone.UUID())
		}}
		if err := f.leave(t.Context(), tr, c.Members[1].ID, c.ID); errs.CodeOf(err) != tt.want {
			t.Errorf("%s: err = %v, want %s", tt.name, err, tt.want)
		}
	}
	if got := f.lefts(t); len(got) != 0 {
		t.Fatalf("member_left events = %+v, want none", got)
	}
}

func TestLeaveCabal_wrapsABrokenMemberTable(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name   string
		before string
		during string
	}{
		{name: "list", before: `ALTER TABLE cabal_members RENAME TO members_gone`},
		{name: "lock", during: `ALTER TABLE cabal_members RENAME TO members_gone`},
		{name: "delete", before: `CREATE FUNCTION refuse() RETURNS trigger LANGUAGE plpgsql AS
			$$ BEGIN RAISE EXCEPTION 'refused'; END $$;
			CREATE TRIGGER refuse BEFORE DELETE ON cabal_members FOR EACH ROW EXECUTE FUNCTION refuse()`},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			f := newAccess(t)
			c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
			tr := &treasuryStub{}
			if tt.before != "" {
				f.exec(t, tt.before)
			}
			if tt.during != "" {
				tr.read = func() { f.exec(t, tt.during) }
			}
			wantErr(t, f.leave(t.Context(), tr, c.Members[1].ID, c.ID), errs.CodeInternal)
		})
	}
}

func TestLeaveCabal_letsAtMostOneOfTwoMembersLeaveAPotWithMoney(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	tr := &treasuryStub{pot: pot(5)}
	var left atomic.Int32
	var wg sync.WaitGroup
	for _, m := range c.Members {
		wg.Go(func() {
			if err := f.leave(t.Context(), tr, m.ID, c.ID); err == nil {
				left.Add(1)
			}
		})
	}
	wg.Wait()
	remaining := len(memberRows(t, f.pool, c))
	if n := int(left.Load()); n > 1 || n+remaining != 2 || len(f.lefts(t)) != n {
		t.Fatalf("%d left, %d remain, %d events; want at most one leave and the pot never orphaned",
			n, remaining, len(f.lefts(t)))
	}
}

func TestDeleteCabalMemberMe_answersNoContentOrTheRefusal(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	routes := adapters.HTTP{Leave: app.NewLeaveCabalHandler(f.uow, f.pool, &treasuryStub{})}
	req := api.DeleteCabalMemberMeRequestObject{Id: c.ID.UUID()}
	if _, err := routes.DeleteCabalMemberMe(t.Context(), req); errs.CodeOf(err) != errs.CodeUnauthorized {
		t.Fatalf("no caller: err = %v, want unauthorized", err)
	}
	if _, err := routes.DeleteCabalMemberMe(as(t.Context(), c.Creator.ID), req); errs.CodeOf(err) !=
		errs.CodeLeaveCreatorWithMembers {
		t.Fatalf("the creator: err = %v, want leave_creator_with_members", err)
	}
	res, err := routes.DeleteCabalMemberMe(as(t.Context(), c.Members[1].ID), req)
	if _, ok := res.(api.DeleteCabalMemberMe204Response); err != nil || !ok {
		t.Fatalf("the member: DeleteCabalMemberMe = %T, %v; want 204", res, err)
	}
}

func TestModule_leavesThroughTheTreasuryReadsItIsGiven(t *testing.T) {
	t.Parallel()
	f := newAccess(t)
	c := testkit.NewCabal(t, f.pool, testkit.WithMembers(2))
	m := cabal.New(module.Deps{
		Pool:  f.pool,
		UoW:   f.uow,
		Clock: f.clock,
		IDs:   f.ids,
		Config: config.Config{
			Privy:    config.Privy{BaseURL: "http://127.0.0.1", VerificationKey: fakes.PrivyVerificationKey()},
			Timeouts: config.Timeouts{Privy: time.Second},
		},
	}, cabal.WithTreasuryReads(&treasuryStub{shares: shares(1)}))
	var routes httpx.Routes
	m.Routes(&routes)
	_, err := routes.DeleteCabalMemberMe(as(t.Context(), c.Members[1].ID),
		api.DeleteCabalMemberMeRequestObject{Id: c.ID.UUID()})
	wantErr(t, err, errs.CodeLeaveHoldsShares)
}
